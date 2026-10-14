package repository

import (
	"context"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testIssuer = "https://issuer.test/dex"

func secureStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "security.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err = s.ConfigureSecurity(context.Background(), []string{"hugofleet-a", "hugofleet-b"}, testIssuer, "admin-subject"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"hugofleet-a", "hugofleet-b"} {
		if err = s.ClusterConnectivity(context.Background(), id, true, time.Now().UnixMilli(), "m4-test"); err != nil {
			t.Fatal(err)
		}
	}
	return s, path
}

// These tests exercise real SQLite authorization/session state. OIDC verification
// belongs to auth and the separate real Dex E2E; IssueSession is its trusted seam.
func identity(t *testing.T, s *Store, sub, email string) (context.Context, Session, string) {
	t.Helper()
	session, token, err := s.IssueSession(domain.WithActor(context.Background(), domain.Actor{RequestID: "login-request"}), testIssuer, sub, email, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	ctx := domain.WithActor(context.Background(), domain.Actor{UserID: session.User.ID, SessionHash: session.Hash, RequestID: "request-" + sub})
	return ctx, session, token
}
func testProfile(tenant, cluster string) domain.TenantProfile {
	return domain.TenantProfile{TenantID: tenant, ClusterID: cluster, Namespace: domain.NamespaceForTenant(tenant), BootstrapUID: "receipt-" + tenant + cluster, Hard: map[string]string{"requests.cpu": "1", "requests.memory": "1Gi", "limits.cpu": "3", "limits.memory": "2Gi", "pods": "8"}, Defaults: map[string]string{"requests.cpu": "100m", "requests.memory": "64Mi", "limits.cpu": "300m", "limits.memory": "128Mi"}}
}
func TestSecurityRoleScopeRevocationAndAtomicIntent(t *testing.T) {
	s, _ := secureStore(t)
	admin, _, _ := identity(t, s, "admin-subject", "admin@example.test")
	owner, own, _ := identity(t, s, "owner-a", "owner@example.test")
	dev, developer, _ := identity(t, s, "developer-a", "dev@example.test")
	other, otherUser, _ := identity(t, s, "owner-b", "other@example.test")
	for _, item := range []struct{ tenant, owner string }{{"team-a", own.User.ID}, {"team-b", otherUser.User.ID}} {
		if err := s.ActivateTenant(admin, item.tenant, item.owner, testProfile(item.tenant, "hugofleet-a")); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetMember(owner, "team-a", developer.User.ID, "Developer"); err != nil {
		t.Fatal(err)
	}
	c := intent("pricing")
	c.TenantID = "team-a"
	c.ClusterID = "hugofleet-a"
	first, err := s.Accept(dev, c, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Accept(dev, c, time.Minute)
	if err != nil || first != again {
		t.Fatalf("dedup=%+v %+v %v", first, again, err)
	}
	detail, err := s.ScopedDetail(dev, "team-a", first.ApplicationID, 100)
	if err != nil || len(detail.Releases) != 1 || detail.Releases[0].CreatedBy != developer.User.ID || detail.Operations[0].RequestID != "request-developer-a" {
		t.Fatalf("trusted actor persistence %+v %v", detail, err)
	}
	_, err = s.ScopedOperation(other, first.OperationID)
	code(t, err, "PERMISSION_DENIED")
	_, err = s.ScopedDetail(dev, "team-b", first.ApplicationID, 100)
	code(t, err, "PERMISSION_DENIED")
	_, err = s.ScopedApps(dev, "team-b", "", 100)
	code(t, err, "PERMISSION_DENIED")
	_, err = s.AuditEvents(dev, "team-b", 100)
	code(t, err, "PERMISSION_DENIED")
	_, err = s.Accept(context.Background(), c, time.Minute)
	code(t, err, "UNAUTHENTICATED")
	cross := c
	cross.TenantID = "team-b"
	_, err = s.Accept(dev, cross, time.Minute)
	code(t, err, "PERMISSION_DENIED")
	if err = s.SetMember(owner, "team-a", developer.User.ID, "Viewer"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Accept(dev, c, time.Minute)
	code(t, err, "PERMISSION_DENIED") // before replay
	if err = s.SetMember(owner, "team-a", developer.User.ID, ""); err != nil {
		t.Fatal(err)
	}
	_, err = s.ScopedOperation(dev, first.OperationID)
	code(t, err, "PERMISSION_DENIED")
	events, err := s.AuditEvents(owner, "team-a", 100)
	if err != nil {
		t.Fatal(err)
	}
	accepted, replayed := 0, 0
	for _, event := range events {
		if event.Action == "CREATE" && event.Outcome == "ACCEPTED" {
			accepted++
		}
		if event.Outcome == "REPLAY" {
			replayed++
		}
		if strings.Contains(event.TargetID, "eyJ") {
			t.Fatal("token in audit")
		}
	}
	if accepted != 1 || replayed != 1 {
		t.Fatalf("audit %d accepted %d replayed %+v", accepted, replayed, events)
	}
	rows, err := s.Detail(context.Background(), "team-a", first.ApplicationID, 100)
	if err != nil || len(rows.Releases) != 1 || len(rows.Operations) != 1 {
		t.Fatalf("denial changed intent %+v %v", rows, err)
	}
}
func TestIdentityEmailAndSessionRevocation(t *testing.T) {
	s, _ := secureStore(t)
	admin, adminSession, adminToken := identity(t, s, "admin-subject", "admin@example.test")
	if !adminSession.User.PlatformAdmin {
		t.Fatal("trusted bootstrap admin not bound")
	}
	_, same, _ := identity(t, s, "admin-subject", "changed@example.test")
	if same.User.ID != adminSession.User.ID || !same.User.PlatformAdmin || same.User.Email != "changed@example.test" {
		t.Fatal("mutable email changed identity")
	}
	_, other, _ := identity(t, s, "different-subject", "changed@example.test")
	if other.User.ID == same.User.ID || other.User.PlatformAdmin {
		t.Fatal("email granted bootstrap privileges")
	}
	session, err := s.Session(context.Background(), adminToken)
	if err != nil || session.Hash == adminToken || session.User.ID != same.User.ID {
		t.Fatal("opaque session not valid")
	}
	if err = s.RevokeSession(admin); err != nil {
		t.Fatal(err)
	}
	_, err = s.Session(context.Background(), adminToken)
	code(t, err, "UNAUTHENTICATED")
	_, err = s.Authorize(admin, "", "PlatformAdmin")
	code(t, err, "UNAUTHENTICATED")
	expiredCtx, expired, expiredToken := identity(t, s, "expiry-user", "expiry@example.test")
	if _, err = s.db.Exec("UPDATE sessions SET expires_at=0 WHERE token_hash=?", expired.Hash); err != nil {
		t.Fatal(err)
	}
	_, err = s.Session(context.Background(), expiredToken)
	code(t, err, "UNAUTHENTICATED")
	_, err = s.CurrentUser(expiredCtx)
	code(t, err, "UNAUTHENTICATED")
	_, _, err = s.IssueSession(context.Background(), "https://wrong.test", same.User.Subject, same.User.Email, time.Now().Add(time.Minute))
	code(t, err, "UNAUTHENTICATED")
}
func TestLoginTransactionsOneTimeBindingAndExpiry(t *testing.T) {
	s, _ := secureStore(t)
	ctx := context.Background()
	for _, mode := range []string{"success", "wrong-browser", "missing-browser", "short-browser", "expired"} {
		state, binding, nonce, verifier := RandomSecret(), RandomSecret(), RandomSecret(), RandomSecret()
		if err := s.BeginLogin(ctx, state, binding, nonce, verifier, time.Now().Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		usedBinding := binding
		if mode == "wrong-browser" {
			usedBinding = RandomSecret()
		}
		if mode == "missing-browser" {
			usedBinding = ""
		}
		if mode == "short-browser" {
			usedBinding = "invalid"
		}
		if mode == "expired" {
			if _, err := s.db.Exec("UPDATE login_transactions SET expires_at=0 WHERE state_hash=?", SecretHash(state)); err != nil {
				t.Fatal(err)
			}
		}
		transaction, err := s.ConsumeLogin(ctx, state, usedBinding)
		if mode == "success" {
			if err != nil || transaction.Nonce != nonce || transaction.Verifier != verifier {
				t.Fatalf("state storage %+v %v", transaction, err)
			}
		} else {
			code(t, err, "UNAUTHENTICATED")
		}
		_, err = s.ConsumeLogin(ctx, state, binding)
		code(t, err, "UNAUTHENTICATED")
	}
}
func TestOwnerProtectionAndQuotaTransactionRollback(t *testing.T) {
	s, _ := secureStore(t)
	admin, _, _ := identity(t, s, "admin-subject", "admin@example.test")
	owner, own, _ := identity(t, s, "owner", "owner@example.test")
	viewer, user, _ := identity(t, s, "viewer", "viewer@example.test")
	p := testProfile("team-a", "hugofleet-a")
	if err := s.ActivateTenant(admin, "Team A", own.User.ID, p); err != nil {
		t.Fatal(err)
	}
	code(t, s.SetMember(owner, "team-a", own.User.ID, "Developer"), "CONFLICT")
	code(t, s.SetMember(admin, "team-a", own.User.ID, ""), "CONFLICT")
	if err := s.SetMember(owner, "team-a", user.User.ID, "Viewer"); err != nil {
		t.Fatal(err)
	}
	code(t, s.SetMember(viewer, "team-a", user.User.ID, "Owner"), "PERMISSION_DENIED")
	code(t, s.SetMember(owner, "team-a", user.User.ID, "PlatformAdmin"), "INVALID_ARGUMENT")
	c := intent("too-large")
	c.TenantID = "team-a"
	c.ClusterID = "hugofleet-a"
	c.Spec.Resources.Requests.CPU = "1"
	c.Spec.Resources.Limits.CPU = "1"
	_, err := s.Accept(owner, c, time.Minute)
	code(t, err, "QUOTA_EXCEEDED")
	apps, err := s.ScopedApps(owner, "team-a", "", 100)
	if err != nil || len(apps) != 0 {
		t.Fatalf("failed estimate persisted app %+v %v", apps, err)
	}
	p.BootstrapUID = "replacement"
	code(t, s.ActivateTenant(admin, "Team A", own.User.ID, p), "OWNERSHIP_CONFLICT")
	profiles, err := s.Profiles(owner, "team-a")
	if err != nil || len(profiles) != 1 || profiles[0].BootstrapUID == "replacement" {
		t.Fatalf("receipt changed %+v %v", profiles, err)
	}
}
func TestSecurityRestartAndLegacyModeGuards(t *testing.T) {
	s, path := secureStore(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.SecurityEnabled() {
		t.Fatal("security marker lost")
	}
	if err = reopened.CheckLocal(context.Background()); err == nil {
		t.Fatal("legacy M2 opened security DB")
	}
	if err = reopened.ConfigureRemote(context.Background(), []string{"hugofleet-a", "hugofleet-b"}); err == nil {
		t.Fatal("legacy M3 opened security DB")
	}
	var tenants, local int
	if err = reopened.db.QueryRow("SELECT count(*) FROM tenants WHERE id='demo'").Scan(&tenants); err != nil {
		t.Fatal(err)
	}
	if err = reopened.db.QueryRow("SELECT count(*) FROM clusters WHERE id='local-kind'").Scan(&local); err != nil {
		t.Fatal(err)
	}
	if tenants != 0 || local != 0 {
		t.Fatal("legacy seed reintroduced")
	}
	if err = reopened.ConfigureSecurity(context.Background(), []string{"hugofleet-a", "hugofleet-b"}, testIssuer, "admin-subject"); err != nil {
		t.Fatal(err)
	}
	if err = reopened.ConfigureSecurity(context.Background(), []string{"hugofleet-a", "hugofleet-b"}, testIssuer, "other"); err == nil {
		t.Fatal("trusted identity silently changed")
	}
	legacy, _ := testStore(t)
	accept(t, legacy, intent("legacy"))
	if err = legacy.ConfigureSecurity(context.Background(), []string{"hugofleet-a", "hugofleet-b"}, testIssuer, "admin-subject"); err == nil {
		t.Fatal("learning history silently upgraded")
	}
}
