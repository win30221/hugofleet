//go:build oidcintegration

package oidctest

import (
	"context"
	"encoding/base64"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/win30221/hugofleet/stages/m4/auth"
	"github.com/win30221/hugofleet/stages/m4/testdriver"
	"golang.org/x/oauth2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealSignedTokenExpiry(t *testing.T) {
	settings, err := testdriver.ReadSettings(filepath.Join(os.Getenv("HF_M4_STATE"), "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 17*time.Minute)
	defer cancel()
	c := settings.OIDC
	account, err := testdriver.Account(settings, "outsider")
	if err != nil {
		t.Fatal(err)
	}
	client := testdriver.BrowserClient()
	flow, provider, err := testdriver.Authorize(ctx, client, c, account)
	if err != nil {
		t.Fatal(err)
	}
	cfg := oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, RedirectURL: c.BaseURL + "/oidc/callback", Endpoint: provider.Endpoint()}
	token, err := cfg.Exchange(oidc.ClientContext(ctx, client), flow.Code, oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		t.Fatal("real token exchange failed")
	}
	raw, _ := token.Extra("id_token").(string)
	verify := provider.Verifier(&oidc.Config{ClientID: c.ClientID})
	verified, err := verify.Verify(oidc.ClientContext(ctx, client), raw)
	if err != nil {
		t.Fatal("real JWT positive control failed")
	}
	if _, err = provider.Verifier(&oidc.Config{ClientID: "wrong-platform-client"}).Verify(oidc.ClientContext(ctx, client), raw); err == nil {
		t.Fatal("wrong audience accepted")
	}
	var discovery struct {
		Keys string `json:"jwks_uri"`
	}
	if err = provider.Claims(&discovery); err != nil || discovery.Keys == "" {
		t.Fatal("actual JWKS discovery unavailable")
	}
	wrongIssuer := oidc.NewVerifier(c.Issuer+"-wrong", oidc.NewRemoteKeySet(oidc.ClientContext(ctx, client), discovery.Keys), &oidc.Config{ClientID: c.ClientID})
	if _, err = wrongIssuer.Verify(oidc.ClientContext(ctx, client), raw); err == nil {
		t.Fatal("wrong issuer accepted")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatal("actual JWT format invalid")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) == 0 {
		t.Fatal("actual signature invalid")
	}
	sig[0] ^= 1
	parts[2] = base64.RawURLEncoding.EncodeToString(sig)
	if _, err = verify.Verify(oidc.ClientContext(ctx, client), strings.Join(parts, ".")); err == nil {
		t.Fatal("tampered signature accepted")
	}
	if _, err = verify.Verify(oidc.ClientContext(ctx, client), "invalid.jwt"); err == nil {
		t.Fatal("malformed JWT accepted")
	}
	t.Log("PASS real Dex JWT wrong audience, issuer, malformed and tampered signature rejected; waiting for actual expiration")
	remaining := time.Until(verified.Expiry)
	if remaining <= 0 || remaining > 16*time.Minute {
		t.Fatal("bounded real JWT expiry test requires local Dex <=15m token lifetime")
	}
	select {
	case <-time.After(remaining + time.Second):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err = verify.Verify(oidc.ClientContext(ctx, client), raw); err == nil {
		t.Fatal("actually expired Dex signed ID Token accepted")
	}
	t.Log("PASS actual signed Dex ID Token expired and rejected; real wall clock, no clock override")
}

func TestRealDexAndHubOIDC(t *testing.T) {
	settings, err := testdriver.ReadSettings(filepath.Join(os.Getenv("HF_M4_STATE"), "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c := settings.OIDC
	account, err := testdriver.Account(settings, "developer-a")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range settings.Accounts {
		s, err := testdriver.LoginHub(ctx, c, a)
		if err != nil {
			t.Fatalf("real login %s: %v", a.Name, err)
		}
		if s.User.PlatformAdmin != (a.Name == "admin") {
			t.Fatalf("claim groups escalated DB role for %s", a.Name)
		}
		if status, _, err := s.Request(ctx, "POST", "/logout", "", nil); err != nil || status != 303 {
			t.Fatalf("logout %s HTTP %d", a.Name, status)
		}
		r, _ := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/api/v1/auth/me", nil)
		r.AddCookie(s.Cookie)
		response, err := testdriver.BrowserClient().Do(r)
		if err != nil {
			t.Fatal("logout replay request failed")
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatalf("revoked platform session reused HTTP %d", response.StatusCode)
		}
		t.Logf("PASS real Code/PKCE + DB role + cookie + logout: %s", a.Name)
	}
	for _, negative := range []string{"nonce", "state", "binding", "missing-binding", "short-binding"} {
		client := testdriver.BrowserClient()
		target, err := testdriver.BeginHubLogin(ctx, client, c)
		if err != nil {
			t.Fatal(err)
		}
		original := target.Query()
		if negative == "nonce" {
			q := target.Query()
			q.Set("nonce", oauth2.GenerateVerifier())
			target.RawQuery = q.Encode()
		}
		callback, err := testdriver.FollowDex(ctx, client, c, target.String(), account)
		if err != nil {
			t.Fatalf("negative %s real Dex flow: %v", negative, err)
		}
		if negative == "state" {
			q := callback.Query()
			q.Set("state", oauth2.GenerateVerifier())
			callback.RawQuery = q.Encode()
		}
		bindingURL, _ := url.Parse(c.BaseURL + "/oidc/callback")
		cookies := client.Jar.Cookies(bindingURL)
		if negative == "binding" {
			client.Jar.SetCookies(bindingURL, []*http.Cookie{{Name: auth.BindingCookie, Value: oauth2.GenerateVerifier(), Path: "/oidc/callback"}})
		}
		if negative == "missing-binding" {
			client.Jar.SetCookies(bindingURL, []*http.Cookie{{Name: auth.BindingCookie, MaxAge: -1, Path: "/oidc/callback"}})
		}
		if negative == "short-binding" {
			client.Jar.SetCookies(bindingURL, []*http.Cookie{{Name: auth.BindingCookie, Value: "invalid", Path: "/oidc/callback"}})
		}
		r, _ := http.NewRequestWithContext(ctx, "GET", callback.String(), nil)
		response, err := client.Do(r)
		if err != nil {
			t.Fatal("callback negative request failed")
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatalf("%s accepted HTTP %d", negative, response.StatusCode)
		}
		if negative == "binding" || negative == "missing-binding" || negative == "short-binding" {
			client.Jar.SetCookies(bindingURL, cookies)
			r, _ = http.NewRequestWithContext(ctx, "GET", callback.String(), nil)
			response, err = client.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 401 {
				t.Fatal("consumed state reusable after incorrect browser binding")
			}
		}
		if original.Get("state") == "" {
			t.Fatal("Hub state absent")
		}
		t.Logf("PASS real signed Code callback rejects %s", negative)
	}
	for _, negative := range []string{"verifier", "redirect"} {
		client := testdriver.BrowserClient()
		flow, provider, err := testdriver.Authorize(ctx, client, c, account)
		if err != nil {
			t.Fatal(err)
		}
		cfg := oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, RedirectURL: c.BaseURL + "/oidc/callback", Endpoint: provider.Endpoint()}
		verifier := flow.Verifier
		if negative == "verifier" {
			verifier = oauth2.GenerateVerifier()
		} else {
			cfg.RedirectURL = c.BaseURL + "/wrong-callback"
		}
		if _, err = cfg.Exchange(oidc.ClientContext(ctx, client), flow.Code, oauth2.VerifierOption(verifier)); err == nil {
			t.Fatalf("real Dex accepted wrong %s", negative)
		}
		t.Logf("PASS real Dex rejects wrong %s", negative)
	}
	client := testdriver.BrowserClient()
	flow, provider, err := testdriver.Authorize(ctx, client, c, account)
	if err != nil {
		t.Fatal(err)
	}
	cfg := oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, RedirectURL: c.BaseURL + "/oidc/callback", Endpoint: provider.Endpoint()}
	token, err := cfg.Exchange(oidc.ClientContext(ctx, client), flow.Code, oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		t.Fatal("real token exchange failed")
	}
	raw, _ := token.Extra("id_token").(string)
	verified, err := provider.Verifier(&oidc.Config{ClientID: c.ClientID}).Verify(oidc.ClientContext(ctx, client), raw)
	if err != nil {
		t.Fatal("real signed token verification failed")
	}
	var claims struct {
		Groups []string `json:"groups"`
	}
	if verified.Claims(&claims) != nil || !strings.Contains(strings.Join(claims.Groups, ","), "PlatformAdmin") {
		t.Fatal("real elevated groups negative control absent")
	}
	for _, secret := range []string{raw, token.AccessToken} {
		r, _ := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/api/v1/auth/me", nil)
		r.Header.Set("Authorization", "Bearer "+secret)
		r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: secret})
		response, err := testdriver.BrowserClient().Do(r)
		if err != nil {
			t.Fatal("token separation request failed")
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatalf("OIDC token used as platform session HTTP %d", response.StatusCode)
		}
	}
	if _, err = cfg.Exchange(oidc.ClientContext(ctx, client), flow.Code, oauth2.VerifierOption(flow.Verifier)); err == nil {
		t.Fatal("real authorization code replay accepted")
	}
	t.Log("PASS actual elevated claim groups ignored; ID/access tokens are not platform sessions; code one-use")
	s, err := testdriver.LoginHub(ctx, c, account)
	if err != nil {
		t.Fatal(err)
	}
	remaining := time.Until(time.UnixMilli(s.ExpiresAt))
	if remaining <= 0 || remaining > 10*time.Second {
		t.Fatal("OIDC integration requires a separate real Hub with --session-ttl <=10s")
	}
	select {
	case <-time.After(remaining + 200*time.Millisecond):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if status, _, err := s.Request(ctx, "GET", "/api/v1/auth/me", "", nil); err != nil || status != 401 {
		t.Fatalf("expired real platform session accepted HTTP %d", status)
	}
	t.Log("PASS actual session expiration using real elapsed time")
}
