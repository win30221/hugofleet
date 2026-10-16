package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m2/hub/service"
	"github.com/win30221/hugofleet/stages/m3/agenttransport"
	"github.com/win30221/hugofleet/stages/m4/auth"
	m5web "github.com/win30221/hugofleet/stages/m5/web"
)

type contractFixture struct {
	dbPath                              string
	store                               *repository.Store
	auth                                *auth.Auth
	admin, owner, member                repository.Session
	adminToken, ownerToken, memberToken string
}

func newContractFixture(t *testing.T) *contractFixture {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "hub.db")
	store, err := repository.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	issuer := "http://127.0.0.1:5556/dex"
	if err = store.ConfigureSecurity(ctx, []string{"hugofleet-a", "hugofleet-b"}, issuer, "admin"); err != nil {
		t.Fatal(err)
	}
	if err = store.ClusterConnectivity(ctx, "hugofleet-a", true, time.Now().UnixMilli(), "contract-test"); err != nil {
		t.Fatal(err)
	}
	f := &contractFixture{store: store, dbPath: dbPath}
	f.auth = &auth.Auth{Config: auth.Config{Issuer: issuer, BaseURL: "http://127.0.0.1:18094", LocalDemo: true}, Store: store}
	for _, item := range []struct {
		name    string
		session *repository.Session
		token   *string
	}{
		{"admin", &f.admin, &f.adminToken}, {"owner", &f.owner, &f.ownerToken}, {"member", &f.member, &f.memberToken},
	} {
		*item.session, *item.token, err = store.IssueSession(ctx, issuer, item.name, item.name+"@example.test", time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
	}
	p, _ := unitTransport{}.TenantProfile(ctx, "hugofleet-a", "team-a")
	if err = store.ActivateTenant(f.actor(f.admin), "Team A", f.owner.User.ID, p); err != nil {
		t.Fatal(err)
	}
	if err = store.SetMember(f.actor(f.admin), "team-a", f.member.User.ID, "Developer"); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *contractFixture) actor(v repository.Session) context.Context {
	return domain.WithActor(context.Background(), domain.Actor{UserID: v.User.ID, SessionHash: v.Hash, RequestID: "contract-test"})
}

func (f *contractFixture) handler(t *testing.T, dashboard bool, transport ProfileTransport) http.Handler {
	t.Helper()
	options := Options{Dashboard: dashboard}
	if dashboard {
		options.Assets = m5web.Assets
	}
	s, err := NewWithOptions(f.auth, &service.Service{Store: f.store, Policy: domain.DefaultPolicy(), Timeout: time.Minute}, transport, options)
	if err != nil {
		t.Fatal(err)
	}
	return s.Handler()
}

func (f *contractFixture) request(h http.Handler, method, path, body string, user repository.Session, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, f.auth.Config.BaseURL+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:12345"
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	}
	r.Header.Set("Origin", f.auth.Config.BaseURL)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", user.CSRF)
	r.Header.Set("Idempotency-Key", repository.RandomSecret())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestMemberRoleMustBeExplicit(t *testing.T) {
	for _, dashboard := range []bool{false, true} {
		t.Run(fmt.Sprint(dashboard), func(t *testing.T) {
			f := newContractFixture(t)
			h := f.handler(t, dashboard, unitTransport{})
			path := "/api/v1/tenants/team-a/members"
			for _, suffix := range []string{"", `,"role":null`, `,"role":"invalid"`} {
				body := fmt.Sprintf(`{"user_id":%q%s}`, f.member.User.ID, suffix)
				w := f.request(h, "POST", path, body, f.owner, f.ownerToken)
				if w.Code != 400 {
					t.Errorf("invalid role %s: HTTP %d", suffix, w.Code)
				}
				role, err := f.store.Authorize(f.actor(f.member), "team-a", "Developer")
				if err != nil || role != "Developer" {
					t.Fatalf("invalid input changed membership: %s %v", role, err)
				}
			}
			w := f.request(h, "POST", path, fmt.Sprintf(`{"user_id":%q,"role":""}`, f.member.User.ID), f.owner, f.ownerToken)
			if w.Code != 200 {
				t.Fatalf("explicit revocation: %d %s", w.Code, w.Body.String())
			}
			if _, err := f.store.Authorize(f.actor(f.member), "team-a", "Viewer"); err == nil {
				t.Fatal("explicit revocation did not remove access")
			}
			w = f.request(h, "POST", path, fmt.Sprintf(`{"user_id":%q,"role":""}`, f.owner.User.ID), f.owner, f.ownerToken)
			if w.Code != 409 {
				t.Fatalf("last Owner protection: HTTP %d", w.Code)
			}
		})
	}
}

func TestReadinessFailureIsJSON(t *testing.T) {
	for _, dashboard := range []bool{false, true} {
		t.Run(fmt.Sprint(dashboard), func(t *testing.T) {
			f := newContractFixture(t)
			h := f.handler(t, dashboard, agenttransport.New([]string{"hugofleet-a", "hugofleet-b"}))
			w := f.request(h, "GET", "/readyz", "", repository.Session{}, "")
			var body struct {
				Error struct {
					Code, Message string
					RequestID     string `json:"request_id"`
				}
			}
			if w.Code != 503 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error.Code != "CLUSTER_OFFLINE" || body.Error.RequestID != w.Header().Get("X-Request-ID") {
				t.Fatalf("readiness contract: HTTP %d, type=%s, body=%s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
			}
		})
	}
}

func TestApplicationListEnvelopeAndCursor(t *testing.T) {
	for _, dashboard := range []bool{false, true} {
		t.Run(fmt.Sprint(dashboard), func(t *testing.T) {
			f := newContractFixture(t)
			h := f.handler(t, dashboard, unitTransport{})
			path := "/api/v1/tenants/team-a/apps"
			var page struct {
				Items []domain.Application `json:"items"`
				Next  string               `json:"next_after"`
			}
			w := f.request(h, "GET", path, "", f.owner, f.ownerToken)
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Items == nil || len(page.Items) != 0 || page.Next != "" {
				t.Fatalf("empty page: %d %s", w.Code, w.Body.String())
			}
			for i := 0; i < 2; i++ {
				body := fmt.Sprintf(`{"name":"app-%d","cluster_id":"hugofleet-a","expected_desired_revision":0,"image":"pricing-api:v1"}`, i)
				if w = f.request(h, "POST", path, body, f.owner, f.ownerToken); w.Code != 202 {
					t.Fatalf("create: %d %s", w.Code, w.Body.String())
				}
			}
			w = f.request(h, "GET", path+"?limit=1", "", f.owner, f.ownerToken)
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Next != page.Items[0].ID {
				t.Fatalf("first page: %d %s", w.Code, w.Body.String())
			}
			first := page.Items[0].ID
			w = f.request(h, "GET", path+"?limit=1&after="+page.Next, "", f.owner, f.ownerToken)
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].ID <= first || page.Next != "" {
				t.Fatalf("last page: %d %s", w.Code, w.Body.String())
			}
			if w = f.request(h, "GET", path+"?limit=101", "", f.owner, f.ownerToken); w.Code != 400 {
				t.Fatalf("unbounded page accepted: %d", w.Code)
			}
		})
	}
}

func TestTenantNameUsesUnicodeCharacters(t *testing.T) {
	f := newContractFixture(t)
	h := f.handler(t, false, unitTransport{})
	for i, name := range []string{strings.Repeat("租", 100), strings.Repeat("a", 100), strings.Repeat("租", 101), strings.Repeat("a", 101)} {
		body := fmt.Sprintf(`{"slug":"name-%d","name":%q,"cluster_id":"hugofleet-a","owner_id":%q}`, i, name, f.owner.User.ID)
		want := 201
		if i >= 2 {
			want = 400
		}
		if w := f.request(h, "POST", "/api/v1/tenants", body, f.admin, f.adminToken); w.Code != want {
			t.Errorf("name case %d: HTTP %d, want %d", i, w.Code, want)
		}
	}
}

func TestManagementListsReachBeyondHundredRecords(t *testing.T) {
	f := newContractFixture(t)
	for i := 0; i < 101; i++ {
		v, _, err := f.store.IssueSession(context.Background(), f.auth.Config.Issuer, fmt.Sprintf("user-%03d", i), fmt.Sprintf("u%d@example.test", i), time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if err = f.store.SetMember(f.actor(f.admin), "team-a", v.User.ID, "Viewer"); err != nil {
			t.Fatal(err)
		}
		p, _ := unitTransport{}.TenantProfile(context.Background(), "hugofleet-a", fmt.Sprintf("tenant-%03d", i))
		if err = f.store.ActivateTenant(f.actor(f.admin), p.TenantID, f.owner.User.ID, p); err != nil {
			t.Fatal(err)
		}
	}
	for _, dashboard := range []bool{false, true} {
		t.Run(fmt.Sprint(dashboard), func(t *testing.T) {
			h := f.handler(t, dashboard, unitTransport{})
			for _, item := range []struct {
				path, id string
				count    int
			}{
				{"/api/v1/users", "id", 104}, {"/api/v1/tenants", "id", 102}, {"/api/v1/tenants/team-a/members", "user_id", 103},
			} {
				path := item.path + "?limit=37"
				seen := map[string]bool{}
				for n := 0; ; n++ {
					if n > 4 {
						t.Fatal("pagination did not terminate")
					}
					w := f.request(h, "GET", path, "", f.admin, f.adminToken)
					var rows []map[string]any
					if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &rows) != nil || len(rows) > 37 {
						t.Fatalf("page %s: HTTP %d %s", path, w.Code, w.Body.String())
					}
					for _, row := range rows {
						id := row[item.id].(string)
						if seen[id] {
							t.Fatalf("duplicate record on %s", item.path)
						}
						seen[id] = true
					}
					link := w.Header().Get("Link")
					if link == "" {
						break
					}
					if !strings.HasPrefix(link, "<") || !strings.HasSuffix(link, `>; rel="next"`) {
						t.Fatalf("invalid next Link: %s", link)
					}
					path = strings.TrimSuffix(strings.TrimPrefix(link, "<"), `>; rel="next"`)
				}
				if len(seen) != item.count {
					t.Errorf("%s reached %d records, want %d", item.path, len(seen), item.count)
				}
				if w := f.request(h, "GET", item.path+"?limit=101", "", f.admin, f.adminToken); w.Code != 400 {
					t.Fatalf("limit not enforced on %s", item.path)
				}
			}
			if w := f.request(h, "GET", "/api/v1/users?after=z", "", f.member, f.memberToken); w.Code != 403 {
				t.Fatalf("non-admin user list access: %d", w.Code)
			}
			w := f.request(h, "GET", "/api/v1/tenants?limit=1", "", f.member, f.memberToken)
			var tenants []domain.Tenant
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &tenants) != nil || len(tenants) != 1 || tenants[0].ID != "team-a" || w.Header().Get("Link") != "" {
				t.Fatal("tenant page leaked foreign membership")
			}
			// The browser must also reach all tenants, Owner options and members.
			root := "/"
			if dashboard {
				root = "/tenants"
			}
			for _, item := range []struct{ path, key, label string }{{root, "tenants_after", "下一頁租戶"}, {root, "users_after", "下一頁 Owner 候選使用者"}, {"/tenants/team-a", "members_after", "下一頁成員"}} {
				w := f.request(h, "GET", item.path, "", f.admin, f.adminToken)
				// Other directories preserve this cursor in their own links. Match
				// the link label so those do not masquerade as another next page.
				re := regexp.MustCompile(`href="([^"]*` + item.key + `=[^"]*)">` + regexp.QuoteMeta(item.label) + `</a>`)
				match := re.FindStringSubmatch(w.Body.String())
				if w.Code != 200 || len(match) != 2 {
					t.Fatalf("missing browser cursor %s: HTTP %d", item.key, w.Code)
				}
				w = f.request(h, "GET", html.UnescapeString(match[1]), "", f.admin, f.adminToken)
				if w.Code != 200 || re.MatchString(w.Body.String()) {
					t.Fatalf("browser final page for %s: HTTP %d", item.key, w.Code)
				}
			}
		})
	}
}

// Retained deleted applications can exceed the 30-live-application limit.
// Seed history in real SQLite; exercise pagination through authenticated HTML.
func TestHTMLApplicationPagination(t *testing.T) {
	for _, tc := range []struct {
		name      string
		dashboard bool
		path      string
		size      int
	}{
		{"m4-tenant", false, "/tenants/team-a?members_after=keep", 100},
		{"m5-tenant", true, "/tenants/team-a?members_after=keep", 100},
		{"m5-filtered", true, "/applications?tenant=team-a&cluster=hugofleet-a&tenants_after=keep", 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newContractFixture(t)
			db, err := sql.Open("sqlite", f.dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			seed := func(i int) {
				t.Helper()
				id := fmt.Sprintf("history-%03d", i)
				if _, err := db.Exec(`INSERT INTO applications(id,tenant_id,cluster_id,name,desired_revision,status) VALUES(?,'team-a','hugofleet-a',?,1,'DELETED')`, id, id); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO releases(id,app_id,revision,spec_snapshot_json,state,created_by,created_at) VALUES(?,?,1,'{}','SUCCEEDED','fixture',1)`, id, id); err != nil {
					t.Fatal(err)
				}
			}
			h := f.handler(t, tc.dashboard, unitTransport{})
			nextPattern := regexp.MustCompile(`href="([^"]*)">下一頁</a>`)
			appPattern := regexp.MustCompile(`href="/tenants/team-a/apps/(history-[0-9]+)"`)
			check := func(path string, count int, wantNext bool) (string, []string) {
				t.Helper()
				w := f.request(h, "GET", path, "", f.owner, f.ownerToken)
				if w.Code != 200 {
					t.Fatalf("GET %s: %d %s", path, w.Code, w.Body.String())
				}
				apps := appPattern.FindAllStringSubmatch(w.Body.String(), -1)
				if len(apps) != count {
					t.Fatalf("GET %s: got %d apps, want %d", path, len(apps), count)
				}
				match := nextPattern.FindStringSubmatch(w.Body.String())
				if (match != nil) != wantNext {
					t.Fatalf("GET %s: next link=%v, want %v", path, match, wantNext)
				}
				ids := []string{}
				for _, app := range apps {
					ids = append(ids, app[1])
				}
				if match == nil {
					return "", ids
				}
				return html.UnescapeString(match[1]), ids
			}
			check(tc.path, 0, false)
			for i := 0; i < tc.size; i++ {
				seed(i)
			}
			check(tc.path, tc.size, false)
			seed(tc.size)
			next, first := check(tc.path, tc.size, true)
			original, _ := url.Parse(tc.path)
			target, err := url.Parse(next)
			if err != nil {
				t.Fatal(err)
			}
			for key, values := range original.Query() {
				if target.Query().Get(key) != values[0] {
					t.Fatalf("lost filter %s in %s", key, next)
				}
			}
			_, last := check(next, 1, false)
			if last[0] <= first[len(first)-1] {
				t.Fatalf("cursor repeats or skips ordering: %v / %v", first, last)
			}
			for i := tc.size + 1; i < tc.size*2; i++ {
				seed(i)
			}
			next, _ = check(tc.path, tc.size, true)
			check(next, tc.size, false)
		})
	}
}
