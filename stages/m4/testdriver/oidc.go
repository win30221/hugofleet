// Package testdriver exercises real local Dex/Hub HTTP flows in Go acceptance
// tests. It never invents provider responses or Kubernetes workload status.
package testdriver

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/win30221/hugofleet/stages/m4/auth"
	"github.com/win30221/hugofleet/stages/m4/democonfig"
	"golang.org/x/net/html"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

func ReadSettings(path string) (democonfig.Settings, error) {
	var v democonfig.Settings
	f, err := os.Open(path)
	if err != nil {
		return v, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 64<<10))
	d.DisallowUnknownFields()
	if err = d.Decode(&v); err != nil {
		return v, fmt.Errorf("invalid private Demo settings")
	}
	v.OIDC.SessionTTL = time.Hour
	return v, v.OIDC.Validate()
}
func Account(v democonfig.Settings, name string) (democonfig.Account, error) {
	for _, a := range v.Accounts {
		if a.Name == name {
			return a, nil
		}
	}
	return democonfig.Account{}, fmt.Errorf("unknown generated Demo account")
}
func BrowserClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

type Flow struct {
	Code, State, Nonce, Verifier string
	Callback                     *url.URL
}

func formValues(n *html.Node) (url.Values, string) {
	values := url.Values{}
	action := ""
	for _, a := range n.Attr {
		if a.Key == "action" {
			action = a.Val
		}
	}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && (node.Data == "input" || node.Data == "button") {
			name, value, kind := "", "", ""
			for _, a := range node.Attr {
				switch a.Key {
				case "name":
					name = a.Val
				case "value":
					value = a.Val
				case "type":
					kind = a.Val
				}
			}
			if name != "" && kind != "submit" {
				values.Set(name, value)
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return values, action
}

// FollowDex stops before the registered Hub callback. The callback is obtained
// from Dex's actual redirect, not synthesized from a known subject or token.
func FollowDex(ctx context.Context, client *http.Client, c auth.Config, target string, account democonfig.Account) (*url.URL, error) {
	issuer, err := url.Parse(c.Issuer)
	if err != nil {
		return nil, err
	}
	callback, err := url.Parse(c.BaseURL + "/oidc/callback")
	if err != nil {
		return nil, err
	}
	method, body := "GET", ""
	for step := 0; step < 12; step++ {
		u, err := url.Parse(target)
		if err != nil {
			return nil, fmt.Errorf("invalid provider redirect")
		}
		if u.Scheme == callback.Scheme && u.Host == callback.Host && u.Path == callback.Path {
			return u, nil
		}
		if u.Scheme != issuer.Scheme || u.Host != issuer.Host || !strings.HasPrefix(u.Path, issuer.Path+"/") {
			return nil, fmt.Errorf("provider redirected outside the configured local issuer/callback")
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		if method == "POST" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("provider HTTP request failed")
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 256<<10))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			return nil, fmt.Errorf("provider response unavailable")
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			next, e := url.Parse(response.Header.Get("Location"))
			if e != nil {
				return nil, fmt.Errorf("invalid provider Location")
			}
			target = u.ResolveReference(next).String()
			method, body = "GET", ""
			continue
		}
		if response.StatusCode != 200 {
			return nil, fmt.Errorf("provider refused flow with HTTP %d", response.StatusCode)
		}
		doc, e := html.Parse(strings.NewReader(string(raw)))
		if e != nil {
			return nil, fmt.Errorf("invalid provider login HTML")
		}
		var loginForm *html.Node
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "form" {
				vals, _ := formValues(n)
				if _, ok := vals["password"]; ok {
					loginForm = n
				}
			}
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				walk(ch)
			}
		}
		walk(doc)
		if loginForm == nil {
			return nil, fmt.Errorf("real Dex password form not found")
		}
		values, action := formValues(loginForm)
		if _, ok := values["login"]; !ok {
			return nil, fmt.Errorf("real Dex login input missing")
		}
		values.Set("login", account.Email)
		values.Set("password", account.Password)
		destination, e := url.Parse(action)
		if e != nil {
			return nil, fmt.Errorf("invalid provider form action")
		}
		target = u.ResolveReference(destination).String()
		method, body = "POST", values.Encode()
	}
	return nil, fmt.Errorf("provider redirect/login step limit exceeded")
}
func Authorize(ctx context.Context, client *http.Client, c auth.Config, account democonfig.Account) (Flow, *oidc.Provider, error) {
	var flow Flow
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), c.Issuer)
	if err != nil {
		return flow, nil, fmt.Errorf("real OIDC discovery unavailable")
	}
	flow.State, flow.Nonce, flow.Verifier = oauth2.GenerateVerifier(), oauth2.GenerateVerifier(), oauth2.GenerateVerifier()
	scopes := []string{oidc.ScopeOpenID, oidc.ScopeEmail, oidc.ScopeProfile}
	if c.LocalDemo {
		scopes = append(scopes, "groups")
	}
	config := oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, RedirectURL: c.BaseURL + "/oidc/callback", Endpoint: provider.Endpoint(), Scopes: scopes}
	callback, err := FollowDex(ctx, client, c, config.AuthCodeURL(flow.State, oidc.Nonce(flow.Nonce), oauth2.S256ChallengeOption(flow.Verifier)), account)
	if err != nil {
		return flow, nil, err
	}
	flow.Callback = callback
	flow.Code = callback.Query().Get("code")
	if callback.Query().Get("state") != flow.State || flow.Code == "" {
		return flow, nil, fmt.Errorf("real provider authorization failed")
	}
	return flow, provider, nil
}
func LearnSubject(ctx context.Context, c auth.Config, account democonfig.Account) (string, error) {
	client := BrowserClient()
	flow, provider, err := Authorize(ctx, client, c, account)
	if err != nil {
		return "", err
	}
	config := oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, RedirectURL: c.BaseURL + "/oidc/callback", Endpoint: provider.Endpoint()}
	token, err := config.Exchange(oidc.ClientContext(ctx, client), flow.Code, oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		return "", fmt.Errorf("real Dex Code/PKCE exchange failed")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return "", fmt.Errorf("real Dex ID Token missing")
	}
	verified, err := provider.Verifier(&oidc.Config{ClientID: c.ClientID}).Verify(oidc.ClientContext(ctx, client), raw)
	if err != nil || verified.Subject == "" || verified.Nonce != flow.Nonce {
		return "", fmt.Errorf("real Dex signature/issuer/audience/expiry/nonce verification failed")
	}
	return verified.Subject, nil
}
