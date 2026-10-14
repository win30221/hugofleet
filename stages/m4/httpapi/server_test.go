package httpapi

import (
	"context"
	"encoding/json"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m2/hub/service"
	"github.com/win30221/hugofleet/stages/m4/auth"
	"github.com/win30221/hugofleet/stages/m4/bootstrap"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type unitTransport struct{}

func (unitTransport) Ping(context.Context) error { return nil }
func (unitTransport) Apply(context.Context, domain.Application, platform.ApplicationSpec) (string, error) {
	return "", nil
}
func (unitTransport) Observe(context.Context, domain.Application, platform.ApplicationSpec) (domain.Observation, error) {
	return domain.Observation{}, nil
}
func (unitTransport) Delete(context.Context, domain.Application) (domain.Observation, error) {
	return domain.Observation{}, nil
}
func (unitTransport) TenantProfile(_ context.Context, cluster, tenant string) (domain.TenantProfile, error) {
	p := bootstrap.DefaultProfile(tenant, cluster)
	p.BootstrapUID = "unit-receipt"
	return p, nil
}
func TestManagementSessionCSRFRoleAndScopedHTML(t *testing.T) {
	ctx := context.Background()
	store, err := repository.Open(ctx, filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	issuer := "http://127.0.0.1:5556/dex"
	if err = store.ConfigureSecurity(ctx, []string{"hugofleet-a", "hugofleet-b"}, issuer, "admin"); err != nil {
		t.Fatal(err)
	}
	for _, cluster := range []string{"hugofleet-a", "hugofleet-b"} {
		if err = store.ClusterConnectivity(ctx, cluster, true, time.Now().UnixMilli(), "unit"); err != nil {
			t.Fatal(err)
		}
	}
	issue := func(sub string) (repository.Session, string) {
		v, token, e := store.IssueSession(ctx, issuer, sub, sub+"@example.test", time.Now().Add(time.Minute))
		if e != nil {
			t.Fatal(e)
		}
		return v, token
	}
	admin, _ := issue("admin")
	owner, ownerToken := issue("owner")
	viewer, viewerToken := issue("viewer")
	_, outsiderToken := issue("outsider")
	actorCtx := func(v repository.Session) context.Context {
		return domain.WithActor(ctx, domain.Actor{UserID: v.User.ID, SessionHash: v.Hash, RequestID: "unit-request"})
	}
	for _, tenant := range []string{"team-a", "team-b"} {
		p, _ := unitTransport{}.TenantProfile(ctx, "hugofleet-a", tenant)
		if err = store.ActivateTenant(actorCtx(admin), tenant, owner.User.ID, p); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.SetMember(actorCtx(owner), "team-a", viewer.User.ID, "Viewer"); err != nil {
		t.Fatal(err)
	}
	a := &auth.Auth{Config: auth.Config{Issuer: issuer, BaseURL: "http://127.0.0.1:18094", LocalDemo: true}, Store: store}
	server, err := New(a, &service.Service{Store: store, Policy: domain.DefaultPolicy(), Timeout: time.Minute}, unitTransport{})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	request := func(method, path, token, csrf, origin, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, a.Config.BaseURL+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:45678"
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
		}
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Origin", origin)
		r.Header.Set("Idempotency-Key", "unit-key")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	state, binding := repository.RandomSecret(), repository.RandomSecret()
	if err = store.BeginLogin(ctx, state, binding, repository.RandomSecret(), repository.RandomSecret(), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if w := request("GET", "/oidc/callback?state="+state+"&code=unused", "", "", "", ""); w.Code != 401 {
		t.Fatalf("missing login binding accepted HTTP %d", w.Code)
	}
	if _, err = store.ConsumeLogin(ctx, state, binding); err == nil {
		t.Fatal("missing binding callback left a reusable login transaction")
	}
	if w := request("GET", "/api/v1/auth/me", "eyJnotAPlatformSession", "", "", ""); w.Code != 401 {
		t.Fatalf("bearer-like token accepted %d", w.Code)
	}
	if w := request("GET", "/api/v1/auth/me", ownerToken, "", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), owner.User.ID) || !strings.Contains(w.Body.String(), "csrf_token") {
		t.Fatalf("session me %d %s", w.Code, w.Body.String())
	}
	// Fetch's origin-header algorithm serializes the Origin of navigation POSTs
	// as null under no-referrer. Protected HTML must support real browser forms,
	// while OAuth URLs keep their query out of subsequent referrers.
	for _, path := range []string{"/", "/tenants/team-a"} {
		w := request("GET", path, ownerToken, "", "", "")
		if w.Code != 200 || w.Header().Get("Referrer-Policy") != "same-origin" {
			t.Fatalf("browser form policy %s: HTTP %d, policy=%q", path, w.Code, w.Header().Get("Referrer-Policy"))
		}
	}
	for _, path := range []string{"/login", "/oidc/callback?state=invalid&code=not-a-secret"} {
		if w := request("GET", path, "", "", "", ""); w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("OAuth referrer protection %s: %q", path, w.Header().Get("Referrer-Policy"))
		}
	}
	for _, item := range []struct{ csrf, origin string }{{"", a.Config.BaseURL}, {repository.RandomSecret(), a.Config.BaseURL}, {owner.CSRF, "https://evil.example.test"}, {owner.CSRF, ""}, {owner.CSRF, "null"}} {
		w := request("POST", "/api/v1/tenants/team-a/apps", ownerToken, item.csrf, item.origin, `{}`)
		if w.Code != 403 || w.Header().Get("X-Request-ID") == "" {
			t.Fatalf("CSRF/origin negative %d", w.Code)
		}
	}
	payload := `{"name":"pricing","cluster_id":"hugofleet-a","expected_desired_revision":0,"image":"pricing-api:v1"}`
	if w := request("POST", "/api/v1/tenants/team-a/apps", viewerToken, viewer.CSRF, a.Config.BaseURL, payload); w.Code != 403 {
		t.Fatalf("Viewer mutated %d %s", w.Code, w.Body.String())
	}
	w := request("POST", "/api/v1/tenants/team-a/apps", ownerToken, owner.CSRF, a.Config.BaseURL, payload)
	if w.Code != 202 {
		t.Fatalf("Owner create %d %s", w.Code, w.Body.String())
	}
	var accepted domain.Accepted
	if err = json.Unmarshal(w.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if w = request("GET", "/api/v1/operations/"+accepted.OperationID, outsiderToken, "", "", ""); w.Code != 403 {
		t.Fatalf("foreign operation exposed %d", w.Code)
	}
	if w = request("GET", "/api/v1/tenants/team-b/apps", viewerToken, "", "", ""); w.Code != 403 {
		t.Fatalf("foreign app list exposed %d", w.Code)
	}
	if w = request("GET", "/api/v1/audit?tenant=team-b", viewerToken, "", "", ""); w.Code != 403 {
		t.Fatalf("foreign audit exposed %d", w.Code)
	}
	if w = request("GET", "/tenants/team-a", viewerToken, "", "", ""); w.Code != 200 || strings.Contains(w.Body.String(), "action=\"/ui/tenants/team-a/apps\"") || !strings.Contains(w.Body.String(), "Viewer") {
		t.Fatalf("Viewer controls/status wrong %d %s", w.Code, w.Body.String())
	}
	if w = request("GET", "/tenants/team-a/apps/"+accepted.ApplicationID, ownerToken, "", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "QUEUED") || !strings.Contains(w.Body.String(), "?confirm=delete") {
		t.Fatalf("real DB detail template %d %s", w.Code, w.Body.String())
	}
	if w = request("GET", "/tenants/team-a/apps/"+accepted.ApplicationID+"?confirm=delete", ownerToken, "", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "確認") || !strings.Contains(w.Body.String(), `name="confirm" value="delete"`) {
		t.Fatalf("delete confirmation template %d %s", w.Code, w.Body.String())
	}
	if w = request("POST", "/logout", ownerToken, owner.CSRF, a.Config.BaseURL, ""); w.Code != 303 {
		t.Fatalf("logout %d", w.Code)
	}
	if w = request("GET", "/api/v1/auth/me", ownerToken, "", "", ""); w.Code != 401 {
		t.Fatalf("logout session reused %d", w.Code)
	}
}
