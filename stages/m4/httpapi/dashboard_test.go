package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m2/hub/service"
	"github.com/win30221/hugofleet/stages/m4/auth"
	"github.com/win30221/hugofleet/stages/m4/bootstrap"
	"github.com/win30221/hugofleet/stages/m5/observability"
	m5web "github.com/win30221/hugofleet/stages/m5/web"
	"go.opentelemetry.io/otel"
)

// Uses real SQLite, HTTP middleware and templates with a synthetic trusted OIDC
// session seam. Worker/provider execution is covered separately by kind E2E.
func TestDashboardScopeTelemetryAndBoundedRefresh(t *testing.T) {
	ctx := context.Background()
	store, err := repository.Open(ctx, filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	issuer := "http://127.0.0.1:5556/dex"
	clusters := []string{"hugofleet-a", "hugofleet-b"}
	if err = store.ConfigureSecurity(ctx, clusters, issuer, "admin"); err != nil {
		t.Fatal(err)
	}
	for _, id := range clusters {
		if err = store.ClusterConnectivity(ctx, id, true, time.Now().UnixMilli(), "m5-unit"); err != nil {
			t.Fatal(err)
		}
	}
	type account struct {
		session repository.Session
		token   string
	}
	users := map[string]account{}
	for _, name := range []string{"admin", "owner-a", "owner-b", "viewer"} {
		v, token, e := store.IssueSession(ctx, issuer, name, name+"@example.test", time.Now().Add(time.Hour))
		if e != nil {
			t.Fatal(e)
		}
		users[name] = account{v, token}
	}
	actor := func(name string) context.Context {
		v := users[name].session
		return domain.WithActor(ctx, domain.Actor{UserID: v.User.ID, SessionHash: v.Hash})
	}
	for _, p := range []struct{ tenant, owner, cluster string }{{"team-a", "owner-a", clusters[0]}, {"team-b", "owner-b", clusters[1]}} {
		profile := bootstrap.DefaultProfile(p.tenant, p.cluster)
		profile.BootstrapUID = "unit-receipt-" + p.tenant
		if err = store.ActivateTenant(actor("admin"), p.tenant, users[p.owner].session.User.ID, profile); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.SetMember(actor("owner-a"), "team-a", users["viewer"].session.User.ID, "Viewer"); err != nil {
		t.Fatal(err)
	}
	old := otel.GetTracerProvider()
	var spans bytes.Buffer
	telemetry, err := observability.New("hub", &spans, clusters)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { telemetry.Provider.Shutdown(ctx); otel.SetTracerProvider(old) }()
	a := &auth.Auth{Config: auth.Config{Issuer: issuer, BaseURL: "http://127.0.0.1:18095", LocalDemo: true}, Store: store}
	server, err := NewWithOptions(a, &service.Service{Store: store, Policy: domain.DefaultPolicy(), Timeout: time.Minute}, unitTransport{}, Options{Dashboard: true, Assets: m5web.Assets, Metrics: telemetry.Metrics.Handler(), Traces: telemetry.Exporter.Records})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	request := func(user, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, a.Config.BaseURL+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:34567"
		r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: users[user].token})
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", a.Config.BaseURL)
		r.Header.Set("X-CSRF-Token", users[user].session.CSRF)
		r.Header.Set("Idempotency-Key", path+user)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	accepted := map[string]domain.Accepted{}
	for _, v := range []struct{ user, tenant, name, cluster string }{{"owner-a", "team-a", "visible-app", clusters[0]}, {"owner-b", "team-b", "private-app", clusters[1]}} {
		body := `{"name":"` + v.name + `","cluster_id":"` + v.cluster + `","expected_desired_revision":0,"image":"pricing-api:v1"}`
		w := request(v.user, "POST", "/api/v1/tenants/"+v.tenant+"/apps", body)
		if w.Code != 202 {
			t.Fatalf("accept=%d %s", w.Code, w.Body.String())
		}
		var intent domain.Accepted
		if err = json.Unmarshal(w.Body.Bytes(), &intent); err != nil {
			t.Fatal(err)
		}
		accepted[v.tenant] = intent
		o, e := store.Operation(ctx, intent.OperationID)
		if e != nil || o.Traceparent == "" || o.RequestID != w.Header().Get("X-Request-ID") {
			t.Fatal("HTTP request trace was not durably correlated")
		}
	}
	for _, path := range []string{"/", "/clusters", "/applications", "/applications?tenant=team-a"} {
		w := request("viewer", "GET", path, "")
		if w.Code != 200 || strings.Contains(w.Body.String(), "private-app") || strings.Contains(w.Body.String(), "/ui/tenants/team-a/apps") {
			t.Fatalf("viewer scope/form error path=%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/applications?tenant=team-b", "/applications?cluster=hugofleet-b", "/metrics", "/traces", "/api/v1/traces"} {
		if w := request("viewer", "GET", path, ""); w.Code != 403 {
			t.Fatalf("viewer access %s=%d", path, w.Code)
		}
	}
	if w := request("admin", "GET", "/metrics", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "hugofleet_agent_connections") {
		t.Fatal("admin metrics unavailable")
	}
	if w := request("owner-a", "GET", "/tenants/team-a", ""); w.Code != 200 || w.Header().Get("Refresh") != "" || !strings.Contains(w.Body.String(), "/ui/tenants/team-a/apps") {
		t.Fatal("editable page refreshed or lost the secured form")
	}
	detail := "/tenants/team-a/apps/" + accepted["team-a"].ApplicationID + "?operation=" + accepted["team-a"].OperationID
	if w := request("owner-a", "GET", detail, ""); w.Code != 200 || w.Header().Get("Refresh") == "" || strings.Contains(w.Body.String(), "/ui/tenants/team-a/apps/") {
		t.Fatal("pending status page has no bounded refresh or overwrites edits")
	}
	if w := request("viewer", "GET", "/?poll=30", ""); w.Header().Get("Refresh") != "" {
		t.Fatal("refresh exceeded thirty requests")
	}
	v, e := store.Overview(actor("viewer"))
	if e != nil || v.Applications != 1 || v.Clusters != 1 || len(v.Recent) != 1 || v.Recent[0].Name != "visible-app" {
		t.Fatalf("overview counted unauthorized/truncated data %+v %v", v, e)
	}
	op, err := store.Operation(ctx, accepted["team-a"].OperationID)
	if err != nil {
		t.Fatal(err)
	}
	app, err := store.App(ctx, accepted["team-a"].ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Finish(ctx, op, app, "SUCCEEDED", "", "unit Worker result"); err != nil {
		t.Fatal(err)
	}
	if w := request("owner-a", "GET", detail, ""); w.Code != 200 || w.Header().Get("Refresh") != "" || strings.Contains(w.Body.String(), "正在等待真實 Worker 回報") {
		t.Fatal("terminal operation still claims to be waiting for Worker or refreshes editable forms")
	}
	if err = store.SetMember(actor("owner-a"), "team-a", users["viewer"].session.User.ID, ""); err != nil {
		t.Fatal(err)
	}
	if w := request("viewer", "GET", "/applications?tenant=team-a", ""); w.Code != 403 {
		t.Fatal("revoked session retained page access")
	}
	if err = telemetry.Provider.ForceFlush(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(spans.String(), users["admin"].token) || strings.Contains(spans.String(), users["owner-a"].session.CSRF) {
		t.Fatal("telemetry contains session/CSRF secrets")
	}
}
