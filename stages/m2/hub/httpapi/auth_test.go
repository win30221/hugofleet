package httpapi

import (
	"context"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m2/hub/service"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestM3TokenLoginCookieOriginAndNetworkBoundary(t *testing.T) {
	store, e := repository.Open(context.Background(), filepath.Join(t.TempDir(), "hub.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	api, e := New(&service.Service{Store: store}, nil, "127.0.0.1:18090")
	if e != nil {
		t.Fatal(e)
	}
	api.Token = strings.Repeat("a", 64)
	api.AllowedHTTPIP = "172.18.0.1"
	call := func(path, method, body, token, ip, origin string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:18090"+path, strings.NewReader(body))
		r.RemoteAddr = ip + ":1234"
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, r)
		return w
	}
	if w := call("/api/v1/clusters", "GET", "", "", "172.18.0.1", "", nil); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := call("/api/v1/clusters", "GET", "", api.Token, "172.18.0.9", "", nil); w.Code != 403 {
		t.Fatal("network bypass", w.Code)
	}
	if w := call("/api/v1/clusters", "GET", "", api.Token, "172.18.0.1", "", nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	login := call("/login", "POST", url.Values{"token": {api.Token}}.Encode(), "", "172.18.0.1", "http://127.0.0.1:18090", nil)
	if login.Code != 303 {
		t.Fatal(login.Code, login.Body)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || strings.Contains(cookies[0].Value, api.Token) {
		t.Fatalf("bad session cookie %+v", cookies)
	}
	if w := call("/api/v1/clusters", "GET", "", "", "172.18.0.1", "", cookies); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := call("/ui/apps", "POST", "", "", "172.18.0.1", "http://evil.local", cookies); w.Code != 403 {
		t.Fatal("origin bypass", w.Code)
	}
	cookies[0].Value = "1." + api.sign("1")
	if w := call("/api/v1/clusters", "GET", "", "", "172.18.0.1", "", cookies); w.Code != 403 {
		t.Fatal("expired session accepted")
	}
	_ = time.Second
}
