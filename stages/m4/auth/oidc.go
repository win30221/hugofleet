// Package auth implements the Hub's OIDC Code + PKCE flow. It never accepts a
// bearer ID/access token as a platform session and never derives roles from claims.
package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"golang.org/x/oauth2"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const SessionCookie = "hf_session"
const BindingCookie = "hf_login_binding"

type Config struct {
	Issuer       string        `json:"issuer"`
	ClientID     string        `json:"client_id"`
	ClientSecret string        `json:"client_secret"`
	BaseURL      string        `json:"base_url"`
	LocalDemo    bool          `json:"local_demo"`
	IssuerDial   string        `json:"issuer_dial,omitempty"`
	HTTPClientIP string        `json:"http_client_ip,omitempty"`
	SessionTTL   time.Duration `json:"-"`
}

func canonicalURL(raw string, base bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" || u.EscapedPath() != u.Path || (u.Scheme != "https" && u.Scheme != "http") || (base && u.Path != "") {
		return nil, fmt.Errorf("invalid canonical OIDC/platform URL")
	}
	if strings.Contains(u.Path, "..") || strings.HasSuffix(u.Path, "/") || (u.Path != "" && u.Path != path.Clean(u.Path)) {
		return nil, fmt.Errorf("canonical URL must not contain traversal or a trailing slash")
	}
	return u, nil
}
func (c Config) Validate() error {
	base, err := canonicalURL(c.BaseURL, true)
	if err != nil {
		return err
	}
	issuer, err := canonicalURL(c.Issuer, false)
	if err != nil {
		return err
	}
	if c.ClientID == "" || len(c.ClientID) > 128 || len(c.ClientSecret) < 32 || c.SessionTTL < time.Second || c.SessionTTL > 8*time.Hour {
		return fmt.Errorf("client credentials and bounded session TTL required")
	}
	if c.LocalDemo {
		if base.Scheme != "http" || issuer.Scheme != "http" || base.Hostname() != "127.0.0.1" || issuer.Hostname() != "127.0.0.1" || base.Port() == "" || issuer.Port() == "" {
			return fmt.Errorf("HTTP Demo is restricted to explicit 127.0.0.1 endpoints")
		}
		if c.HTTPClientIP != "" {
			ip := net.ParseIP(c.HTTPClientIP)
			if ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) {
				return fmt.Errorf("exact private Docker management gateway IP required")
			}
		}
		if c.IssuerDial != "" {
			host, port, e := net.SplitHostPort(c.IssuerDial)
			ip := net.ParseIP(host)
			if e != nil || port == "" || ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) {
				return fmt.Errorf("Demo issuer dial override must be a private IP:port")
			}
		}
	} else if base.Scheme != "https" || issuer.Scheme != "https" || c.IssuerDial != "" || c.HTTPClientIP != "" {
		return fmt.Errorf("normal OIDC configuration requires HTTPS and no Demo network overrides")
	}
	return nil
}

type limitedBody struct {
	io.Reader
	closer io.Closer
}

func (b limitedBody) Close() error { return b.closer.Close() }

type issuerTransport struct {
	base   *http.Transport
	issuer *url.URL
}

// Require the wire URL to remain canonical before a server or proxy can clean
// dot segments, encoded slashes or repeated slashes into another issuer path.
func trustedEndpoint(issuer, endpoint *url.URL) bool {
	return endpoint != nil && endpoint.Scheme == issuer.Scheme && endpoint.Host == issuer.Host && endpoint.User == nil && endpoint.Opaque == "" && endpoint.RawPath == "" && endpoint.EscapedPath() == endpoint.Path && endpoint.RawQuery == "" && endpoint.Fragment == "" && endpoint.Path == path.Clean(endpoint.Path) && strings.HasPrefix(endpoint.Path, issuer.Path+"/")
}

func (t issuerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !trustedEndpoint(t.issuer, r.URL) {
		return nil, fmt.Errorf("OIDC backchannel endpoint is outside the trusted issuer")
	}
	resp, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	resp.Body = limitedBody{io.LimitReader(resp.Body, 512<<10), resp.Body}
	return resp, nil
}

type Auth struct {
	Config   Config
	Store    *repository.Store
	client   *http.Client
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func New(ctx context.Context, c Config, store *repository.Store) (*Auth, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !store.SecurityEnabled() {
		return nil, fmt.Errorf("OIDC requires a security database")
	}
	issuer, _ := url.Parse(c.Issuer)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if c.IssuerDial != "" {
		dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == issuer.Host {
				addr = c.IssuerDial
			}
			return dialer.DialContext(ctx, network, addr)
		}
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: issuerTransport{transport, issuer}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), c.Issuer)
	if err != nil {
		return nil, fmt.Errorf("trusted OIDC discovery unavailable or invalid")
	}
	var discovery struct {
		Auth  string   `json:"authorization_endpoint"`
		Token string   `json:"token_endpoint"`
		JWKS  string   `json:"jwks_uri"`
		PKCE  []string `json:"code_challenge_methods_supported"`
	}
	if err = provider.Claims(&discovery); err != nil {
		return nil, fmt.Errorf("invalid OIDC discovery")
	}
	for _, endpoint := range []string{discovery.Auth, discovery.Token, discovery.JWKS} {
		u, e := url.Parse(endpoint)
		if e != nil || !trustedEndpoint(issuer, u) {
			return nil, fmt.Errorf("OIDC endpoint outside configured issuer origin/path")
		}
	}
	s256 := false
	for _, v := range discovery.PKCE {
		if v == "S256" {
			s256 = true
		}
	}
	if !s256 {
		return nil, fmt.Errorf("OIDC provider must advertise S256 PKCE")
	}
	scopes := []string{oidc.ScopeOpenID, oidc.ScopeEmail, oidc.ScopeProfile}
	if c.LocalDemo {
		scopes = append(scopes, "groups")
	}
	return &Auth{Config: c, Store: store, client: client, oauth: oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: c.BaseURL + "/oidc/callback", Scopes: scopes}, verifier: provider.Verifier(&oidc.Config{ClientID: c.ClientID, SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256}})}, nil
}
func (a *Auth) Begin(ctx context.Context) (string, string, error) {
	state, binding, nonce, verifier := repository.RandomSecret(), repository.RandomSecret(), repository.RandomSecret(), oauth2.GenerateVerifier()
	if err := a.Store.BeginLogin(ctx, state, binding, nonce, verifier, time.Now().Add(5*time.Minute)); err != nil {
		return "", "", err
	}
	return a.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), binding, nil
}
func (a *Auth) Complete(ctx context.Context, state, binding, code string) (repository.Session, string, error) {
	var empty repository.Session
	transaction, err := a.Store.ConsumeLogin(ctx, state, binding)
	if err != nil {
		return empty, "", err
	}
	if code == "" || len(code) > 4096 {
		return empty, "", domain.Problem("UNAUTHENTICATED", "missing or invalid authorization code")
	}
	token, err := a.oauth.Exchange(oidc.ClientContext(ctx, a.client), code, oauth2.VerifierOption(transaction.Verifier))
	if err != nil {
		return empty, "", domain.Problem("UNAUTHENTICATED", "authorization code or PKCE verification failed")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok || raw == "" || len(raw) > 64<<10 {
		return empty, "", domain.Problem("UNAUTHENTICATED", "ID Token missing or invalid")
	}
	verified, err := a.verifier.Verify(oidc.ClientContext(ctx, a.client), raw)
	if err != nil {
		return empty, "", domain.Problem("UNAUTHENTICATED", "ID Token issuer, audience, signature or expiration invalid")
	}
	return a.issue(ctx, verified, transaction.Nonce, token.AccessToken)
}
func (a *Auth) issue(ctx context.Context, verified *oidc.IDToken, nonce, accessToken string) (repository.Session, string, error) {
	var empty repository.Session
	if subtle.ConstantTimeCompare([]byte(verified.Nonce), []byte(nonce)) != 1 {
		return empty, "", domain.Problem("UNAUTHENTICATED", "ID Token nonce does not match login transaction")
	}
	if verified.AccessTokenHash != "" && verified.VerifyAccessToken(accessToken) != nil {
		return empty, "", domain.Problem("UNAUTHENTICATED", "ID Token access-token hash mismatch")
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := verified.Claims(&claims); err != nil {
		return empty, "", domain.Problem("UNAUTHENTICATED", "invalid ID Token claims")
	}
	expires := time.Now().Add(a.Config.SessionTTL)
	if verified.Expiry.Before(expires) {
		expires = verified.Expiry
	}
	return a.Store.IssueSession(ctx, verified.Issuer, verified.Subject, claims.Email, expires)
}

// VerifyToken is used by security tests/bootstrap with an actual provider's JWKS.
// It verifies an ID Token, but deliberately cannot create a platform session.
func (a *Auth) VerifyToken(ctx context.Context, raw string) (*oidc.IDToken, error) {
	return a.verifier.Verify(oidc.ClientContext(ctx, a.client), raw)
}
func (a *Auth) Cookie(name, value, path string, expiry time.Time, sameSite http.SameSite) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: path, HttpOnly: true, Secure: !a.Config.LocalDemo, SameSite: sameSite, Expires: expiry, MaxAge: max(1, int(time.Until(expiry).Seconds()))}
}
func CSRFValid(expected, actual string) bool {
	return len(actual) == 43 && subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}
func PKCEChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
