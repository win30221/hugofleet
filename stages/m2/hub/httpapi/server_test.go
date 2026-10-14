package httpapi

import (
	"context"
	"encoding/json"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m2/hub/service"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Contract tests exercise real HTTP and SQLite; no rollout is claimed here.
// Kubernetes readiness and the UI deployment lifecycle are tested in kind E2E.
func TestHTTPContractsAndUIProtection(t *testing.T) {
	ctx := context.Background()
	store, e := repository.Open(ctx, filepath.Join(t.TempDir(), "hub.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if e = store.Connectivity(ctx, true); e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	api, e := New(&service.Service{Store: store, Policy: domain.DefaultPolicy(), Timeout: time.Minute}, nil, listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewUnstartedServer(api.Handler())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	client := server.Client()
	client.Timeout = 5 * time.Second
	payload := `{"name":"pricing-api","cluster_id":"local-kind","image":"pricing-api:v1","expected_desired_revision":0}`
	call := func(path, body, key, contentType string) (int, []byte) {
		t.Helper()
		r, _ := http.NewRequest("POST", server.URL+path, strings.NewReader(body))
		r.Header.Set("Content-Type", contentType)
		r.Header.Set("Idempotency-Key", key)
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		if res.Header.Get("X-Request-ID") == "" {
			t.Fatal("missing request ID")
		}
		return res.StatusCode, raw
	}
	path := "/api/v1/tenants/demo/apps"
	status, _ := call(path, payload, "key", "text/plain")
	if status != 400 {
		t.Fatal(status)
	}
	status, _ = call(path, strings.TrimSuffix(payload, "}")+`,"yaml":"anything"}`, "key", "application/json")
	if status != 400 {
		t.Fatal("unknown field accepted")
	}
	status, _ = call(path, payload+` {}`, "key", "application/json")
	if status != 400 {
		t.Fatal("trailing JSON accepted")
	}
	status, _ = call(path, payload, "", "application/json")
	if status != 400 {
		t.Fatal("missing idempotency accepted")
	}
	status, raw := call(path, payload, "key", "application/json")
	if status != 202 {
		t.Fatalf("%d %s", status, raw)
	}
	var first domain.Accepted
	if e = json.Unmarshal(raw, &first); e != nil {
		t.Fatal(e)
	}
	status, raw = call(path, payload, "key", "application/json")
	var second domain.Accepted
	json.Unmarshal(raw, &second)
	if status != 202 || first.OperationID != second.OperationID {
		t.Fatal("duplicate operation")
	}
	status, _ = call(path, strings.ReplaceAll(payload, "v1", "v2-good"), "key", "application/json")
	if status != 409 {
		t.Fatal("changed payload accepted")
	}
	for _, path := range []string{"/api/v1/tenants/demo/apps?limit=101", "/api/v1/tenants/demo/apps?limit=-1"} {
		res, e := client.Get(server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Fatal("unbounded pagination accepted")
		}
	}
	jar, _ := cookiejar.New(nil)
	client.Jar = jar
	res, e := client.Get(server.URL + "/")
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "部署 revision 1") || !strings.Contains(string(body), "csrf") {
		t.Fatalf("template did not render: %s", body)
	}
	cookies := res.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("insecure CSRF cookie")
	}
	res, e = client.PostForm(server.URL+"/ui/apps", url.Values{"csrf": {"wrong"}})
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("invalid CSRF accepted")
	}
	res, e = client.PostForm(server.URL+"/ui/apps/"+first.ApplicationID+"/delete", url.Values{"csrf": {cookies[0].Value}, "expected_desired_revision": {"1"}, "idempotency_key": {"unconfirmed-delete"}})
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal("unconfirmed delete was not rejected before mutation")
	}
	for _, tc := range []struct{ host, origin, site string }{{"evil.test", "", ""}, {"", "https://evil.test", ""}, {"", "", "cross-site"}} {
		r, _ := http.NewRequest("POST", server.URL+path, strings.NewReader(payload))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "attack")
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		if tc.host != "" {
			r.Host = tc.host
		}
		res, e = client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatal("cross-origin/Host accepted")
		}
	}
	d, e := store.Detail(ctx, domain.TenantID, first.ApplicationID, 100)
	if e != nil || len(d.Releases) != 1 {
		t.Fatal("rejected requests mutated DB")
	}
	if e = store.Observe(ctx, d.Application, domain.Observation{ErrorCode: "APPLICATION_MISSING", Message: "Application CR is missing <script>"}); e != nil {
		t.Fatal(e)
	}
	res, e = client.Get(server.URL + "/apps/" + first.ApplicationID)
	if e != nil {
		t.Fatal(e)
	}
	body, e = io.ReadAll(res.Body)
	res.Body.Close()
	if e != nil {
		t.Fatal(e)
	}
	if res.StatusCode != 200 || !strings.Contains(string(body), "APPLICATION_MISSING: Application CR is missing &lt;script&gt;") || strings.Contains(string(body), "missing <script>") {
		t.Fatalf("current observation reason missing or unescaped in UI: %s", body)
	}
}
