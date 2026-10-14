package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A cryptographic provider fixture isolates JWT/discovery negatives in Unit.
// The separate M4 kind/OIDC E2E must still use real Dex Code + PKCE.
func TestSignedTokenVerificationNonceAndUntrustedRoleClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	provider := httptest.NewServer(mux)
	defer provider.Close()
	issuer := provider.URL + "/dex"
	mux.HandleFunc("GET /dex/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/auth", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}, "code_challenge_methods_supported": []string{"S256"}})
	})
	mux.HandleFunc("GET /dex/keys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "unit-key", Algorithm: "RS256", Use: "sig"}}})
	})
	ctx := context.Background()
	store, err := repository.Open(ctx, filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.ConfigureSecurity(ctx, []string{"hugofleet-a", "hugofleet-b"}, issuer, "trusted-admin"); err != nil {
		t.Fatal(err)
	}
	a, err := New(ctx, Config{Issuer: issuer, BaseURL: "http://127.0.0.1:18094", ClientID: "hugofleet", ClientSecret: strings.Repeat("x", 32), LocalDemo: true, SessionTTL: time.Hour}, store)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(signingKey *rsa.PrivateKey, iss, subject, audience, nonce string, expiry time.Time) string {
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: signingKey}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "unit-key"))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := jwt.Signed(signer).Claims(jwt.Claims{Issuer: iss, Subject: subject, Audience: jwt.Audience{audience}, Expiry: jwt.NewNumericDate(expiry), IssuedAt: jwt.NewNumericDate(time.Now())}).Claims(map[string]any{"nonce": nonce, "email": "person@example.test", "role": "PlatformAdmin", "groups": []string{"platform-admin", "Owner"}}).Serialize()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	good := sign(key, issuer, "person", "hugofleet", "expected-nonce", time.Now().Add(time.Hour))
	token, err := a.VerifyToken(ctx, good)
	if err != nil {
		t.Fatal(err)
	}
	v, opaque, err := a.issue(ctx, token, "expected-nonce", "")
	if err != nil || v.User.PlatformAdmin || v.User.Subject != "person" || opaque == good {
		t.Fatalf("role claims trusted or session failure %+v %v", v.User, err)
	}
	if _, _, err = a.issue(ctx, token, "different-nonce", ""); err == nil {
		t.Fatal("nonce mismatch accepted")
	}
	for _, raw := range []string{sign(key, "https://wrong.test", "person", "hugofleet", "expected-nonce", time.Now().Add(time.Hour)), sign(key, issuer, "person", "other-client", "expected-nonce", time.Now().Add(time.Hour)), sign(key, issuer, "person", "hugofleet", "expected-nonce", time.Now().Add(-time.Minute)), "not-a-jwt"} {
		if _, err = a.VerifyToken(ctx, raw); err == nil {
			t.Fatal("invalid issuer/audience/expiry/JWT accepted")
		}
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.VerifyToken(ctx, sign(otherKey, issuer, "person", "hugofleet", "expected-nonce", time.Now().Add(time.Hour))); err == nil {
		t.Fatal("untrusted signature accepted")
	}
	target, binding, err := a.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") != a.Config.BaseURL+"/oidc/callback" || q.Get("nonce") == "" || q.Get("state") == "" {
		t.Fatal("Code/PKCE/nonce/redirect flow incomplete")
	}
	transaction, err := store.ConsumeLogin(ctx, q.Get("state"), binding)
	if err != nil || PKCEChallenge(transaction.Verifier) != q.Get("code_challenge") || transaction.Nonce != q.Get("nonce") {
		t.Fatalf("server transaction %v", err)
	}
	_, _, err = a.Complete(ctx, q.Get("state"), binding, "any-code")
	var problem *domain.Error
	if !errors.As(err, &problem) || problem.Code != "UNAUTHENTICATED" {
		t.Fatal("consumed state replay accepted")
	}
}
func TestConfigAndCookieSecurity(t *testing.T) {
	valid := Config{Issuer: "http://127.0.0.1:5556/dex", BaseURL: "http://127.0.0.1:18094", ClientID: "hugofleet", ClientSecret: strings.Repeat("x", 32), LocalDemo: true, SessionTTL: time.Hour}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Config){func(c *Config) { c.BaseURL = "http://0.0.0.0:18094" }, func(c *Config) { c.Issuer = "http://example.com/dex" }, func(c *Config) { c.LocalDemo = false }, func(c *Config) { c.IssuerDial = "8.8.8.8:5556" }, func(c *Config) { c.BaseURL += "/callback" }, func(c *Config) { c.ClientSecret = "short" }, func(c *Config) { c.SessionTTL = 9 * time.Hour }} {
		c := valid
		change(&c)
		if c.Validate() == nil {
			t.Fatal("unsafe OIDC configuration accepted")
		}
	}
	a := &Auth{Config: valid}
	cookie := a.Cookie(SessionCookie, "opaque", "/", time.Now().Add(time.Hour), http.SameSiteStrictMode)
	if !cookie.HttpOnly || cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("Demo cookie contract")
	}
	valid.LocalDemo = false
	valid.BaseURL = "https://hub.example.test"
	valid.Issuer = "https://issuer.example.test/dex"
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	a.Config = valid
	if !a.Cookie(SessionCookie, "opaque", "/", time.Now().Add(time.Hour), http.SameSiteStrictMode).Secure {
		t.Fatal("HTTPS session omitted Secure")
	}
	csrf := repository.RandomSecret()
	if !CSRFValid(csrf, csrf) || CSRFValid(csrf, repository.RandomSecret()) || CSRFValid(csrf, "") {
		t.Fatal("CSRF equality contract")
	}
}

func TestLocalDemoGatewayIsPrivate(t *testing.T) {
	valid := Config{Issuer: "http://127.0.0.1:5556/dex", BaseURL: "http://127.0.0.1:18094", ClientID: "hugofleet", ClientSecret: strings.Repeat("x", 32), LocalDemo: true, SessionTTL: time.Hour}
	for _, gateway := range []string{"203.0.113.23", "2001:db8::1"} {
		t.Run(gateway, func(t *testing.T) {
			c := valid
			c.HTTPClientIP = gateway
			if c.Validate() == nil {
				t.Fatal("public Demo management gateway accepted")
			}
		})
	}
	for _, gateway := range []string{"", "127.0.0.1", "172.19.0.1", "10.0.0.1", "192.168.1.1", "fd00::1"} {
		t.Run("private-"+gateway, func(t *testing.T) {
			c := valid
			c.HTTPClientIP = gateway
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDiscoveryRejectsNoncanonicalEndpointPaths(t *testing.T) {
	for _, endpoint := range []string{"authorization_endpoint", "token_endpoint", "jwks_uri"} {
		for _, suffix := range []string{"/../outside", "/%2e%2e/outside", "//outside", "/./outside", "/%2foutside", "/%252e%252e/outside", "/%252foutside", "/%5coutside", "/%5Coutside", "/space%20outside"} {
			t.Run(endpoint+suffix, func(t *testing.T) {
				mux := http.NewServeMux()
				provider := httptest.NewServer(mux)
				defer provider.Close()
				issuer := provider.URL + "/dex"
				mux.HandleFunc("GET /dex/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
					claims := map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/auth", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}, "code_challenge_methods_supported": []string{"S256"}}
					claims[endpoint] = issuer + suffix
					if err := json.NewEncoder(w).Encode(claims); err != nil {
						t.Error(err)
					}
				})
				ctx := context.Background()
				store, err := repository.Open(ctx, filepath.Join(t.TempDir(), "hub.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				if err = store.ConfigureSecurity(ctx, []string{"hugofleet-a", "hugofleet-b"}, issuer, "trusted-admin"); err != nil {
					t.Fatal(err)
				}
				_, err = New(ctx, Config{Issuer: issuer, BaseURL: "http://127.0.0.1:18094", ClientID: "hugofleet", ClientSecret: strings.Repeat("x", 32), LocalDemo: true, SessionTTL: time.Hour}, store)
				if err == nil {
					t.Fatal("discovery accepted a noncanonical or escaped wire path")
				}
				if !strings.Contains(err.Error(), "OIDC endpoint outside configured issuer origin/path") {
					t.Fatalf("expected endpoint boundary rejection, got %v", err)
				}
			})
		}
	}
}
