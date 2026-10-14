package testdriver

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m4/auth"
	"github.com/win30221/hugofleet/stages/m4/democonfig"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type ManagementSession struct {
	Client    *http.Client
	Config    auth.Config
	User      domain.User
	CSRF      string
	ExpiresAt int64
	Cookie    *http.Cookie
}

// BeginHubLogin obtains the authorization URL and browser binding from the real
// Hub. Test code may alter the URL to exercise nonce/PKCE/redirect rejection.
func BeginHubLogin(ctx context.Context, client *http.Client, c auth.Config) (*url.URL, error) {
	r, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/oidc/login", nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(r)
	if err != nil {
		return nil, fmt.Errorf("real Hub login unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 303 {
		return nil, fmt.Errorf("Hub login HTTP %d", response.StatusCode)
	}
	u, err := url.Parse(response.Header.Get("Location"))
	if err != nil || u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("nonce") == "" || u.Query().Get("state") == "" {
		return nil, fmt.Errorf("Hub did not initiate nonce/state/PKCE login")
	}
	return u, nil
}
func LoginHub(ctx context.Context, c auth.Config, account democonfig.Account) (*ManagementSession, error) {
	client := BrowserClient()
	target, err := BeginHubLogin(ctx, client, c)
	if err != nil {
		return nil, err
	}
	callback, err := FollowDex(ctx, client, c, target.String(), account)
	if err != nil {
		return nil, err
	}
	r, _ := http.NewRequestWithContext(ctx, "GET", callback.String(), nil)
	response, err := client.Do(r)
	if err != nil {
		return nil, fmt.Errorf("Hub callback request failed")
	}
	response.Body.Close()
	if response.StatusCode != 303 {
		return nil, fmt.Errorf("Hub rejected real Code/PKCE callback (HTTP %d)", response.StatusCode)
	}
	s := &ManagementSession{Client: client, Config: c}
	for _, cookie := range response.Cookies() {
		if cookie.Name == auth.SessionCookie {
			s.Cookie = cookie
		}
	}
	if s.Cookie == nil || !s.Cookie.HttpOnly || s.Cookie.SameSite != http.SameSiteStrictMode || s.Cookie.Secure != !c.LocalDemo {
		return nil, fmt.Errorf("session cookie contract violated")
	}
	var me struct {
		User      domain.User `json:"user"`
		CSRF      string      `json:"csrf_token"`
		ExpiresAt int64       `json:"expires_at"`
	}
	status, raw, err := s.Request(ctx, "GET", "/api/v1/auth/me", "", nil)
	if err != nil || status != 200 || json.Unmarshal(raw, &me) != nil || me.User.ID == "" || me.CSRF == "" {
		return nil, fmt.Errorf("real platform session unavailable")
	}
	s.User, s.CSRF, s.ExpiresAt = me.User, me.CSRF, me.ExpiresAt
	return s, nil
}
func (s *ManagementSession) Request(ctx context.Context, method, path, key string, body any) (int, []byte, error) {
	raw := []byte{}
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, s.Config.BaseURL+path, strings.NewReader(string(raw)))
	if err != nil {
		return 0, nil, err
	}
	if method != "GET" && method != "HEAD" {
		r.Header.Set("Origin", s.Config.BaseURL)
		r.Header.Set("X-CSRF-Token", s.CSRF)
		r.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	response, err := s.Client.Do(r)
	if err != nil {
		return 0, nil, fmt.Errorf("management request failed")
	}
	defer response.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(response.Body, 512<<10))
	return response.StatusCode, raw, err
}
