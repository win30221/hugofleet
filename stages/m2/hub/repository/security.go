package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"k8s.io/apimachinery/pkg/api/resource"
	"net/url"
	"strings"
	"time"
)

// ConfigureSecurity is a trusted deployment bootstrap, never an HTTP operation.
// Existing learning data is not upgraded into a multi-tenant security database.
func (s *Store) ConfigureSecurity(ctx context.Context, ids []string, issuer, adminSubject string) error {
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || adminSubject == "" || len(adminSubject) > 512 || len(ids) != 2 || ids[0] == ids[1] {
		return fmt.Errorf("exact OIDC identity and two distinct registered clusters required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldIssuer, oldSub string
	err = tx.QueryRowContext(ctx, "SELECT issuer,admin_subject FROM security_settings WHERE id=1").Scan(&oldIssuer, &oldSub)
	if err == nil {
		if oldIssuer != issuer || oldSub != adminSubject {
			return fmt.Errorf("security database belongs to a different trusted OIDC configuration")
		}
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM clusters WHERE id NOT IN (?,?)", ids[0], ids[1]).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("security database belongs to a different cluster registry")
		}
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM clusters").Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			return fmt.Errorf("incomplete security cluster registry")
		}
		s.secure = true
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var apps, remote int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM applications").Scan(&apps); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM clusters WHERE id!=?", domain.ClusterID).Scan(&remote); err != nil {
		return err
	}
	if apps != 0 || remote != 0 {
		return fmt.Errorf("M4 requires a fresh database, not M2/M3 learning history")
	}
	for _, statement := range []string{"DELETE FROM tenants", "DELETE FROM clusters"} {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	now := time.Now().UnixMilli()
	for _, id := range ids {
		if id == "" {
			return fmt.Errorf("empty cluster identity")
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO clusters(id,name,state,last_seen_at,registered_at) VALUES(?,?,'DISCONNECTED',0,?)", id, id, now); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO security_settings VALUES(1,?,?)", issuer, adminSubject); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,issuer,oidc_subject,platform_admin,created_at) VALUES(?,?,?,1,?)", uuid.NewString(), issuer, adminSubject, now); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.secure = true
	return nil
}
func (s *Store) SecurityEnabled() bool { return s.secure }

type execQuerier interface {
	querier
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func authorizedUser(ctx context.Context, q querier) (domain.User, error) {
	actor := domain.ActorContext(ctx)
	var u domain.User
	if actor.UserID == "" || actor.SessionHash == "" {
		return u, domain.Problem("UNAUTHENTICATED", "valid platform session required")
	}
	err := q.QueryRowContext(ctx, "SELECT u.id,u.issuer,u.oidc_subject,u.email,u.platform_admin,u.created_at FROM users u JOIN sessions s ON s.user_id=u.id WHERE u.id=? AND s.token_hash=? AND s.expires_at>?", actor.UserID, actor.SessionHash, time.Now().UnixMilli()).Scan(&u.ID, &u.Issuer, &u.Subject, &u.Email, &u.PlatformAdmin, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, domain.Problem("UNAUTHENTICATED", "session expired or revoked")
	}
	return u, err
}
func authorize(ctx context.Context, q querier, tenant, minimum string) (string, error) {
	u, err := authorizedUser(ctx, q)
	if err != nil {
		return "", err
	}
	if u.PlatformAdmin {
		return "PlatformAdmin", nil
	}
	if minimum == "PlatformAdmin" || tenant == "" {
		return "", domain.Problem("PERMISSION_DENIED", "platform administrator required")
	}
	var role string
	err = q.QueryRowContext(ctx, "SELECT role FROM tenant_memberships WHERE tenant_id=? AND user_id=?", tenant, u.ID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.Problem("PERMISSION_DENIED", "tenant is outside your authorized scope")
	}
	if err != nil {
		return "", err
	}
	levels := map[string]int{"Viewer": 1, "Developer": 2, "Owner": 3}
	if levels[minimum] == 0 || levels[role] < levels[minimum] {
		return "", domain.Problem("PERMISSION_DENIED", "tenant role does not permit this action")
	}
	return role, nil
}
func (s *Store) Authorize(ctx context.Context, tenant, minimum string) (string, error) {
	if !s.secure {
		return "", domain.Problem("PERMISSION_DENIED", "OIDC security mode required")
	}
	return authorize(ctx, s.db, tenant, minimum)
}
func (s *Store) CurrentUser(ctx context.Context) (domain.User, error) {
	return authorizedUser(ctx, s.db)
}

func audit(ctx context.Context, q execQuerier, tenant, action, targetType, targetID, outcome string) error {
	a := domain.ActorContext(ctx)
	_, err := q.ExecContext(ctx, "INSERT INTO audit_events VALUES(?,?,?,?,?,?,?,?,?)", uuid.NewString(), a.UserID, tenant, action, targetType, targetID, outcome, a.RequestID, time.Now().UnixMilli())
	return err
}

// Audit contains fixed event fields, never an HTTP body, token, callback URL or secret.
func (s *Store) Audit(ctx context.Context, tenant, action, targetType, targetID, outcome string) error {
	return audit(ctx, s.db, tenant, action, targetType, targetID, outcome)
}
func (s *Store) AuditEvents(ctx context.Context, tenant string, limit int) ([]domain.AuditEvent, error) {
	minimum := "Viewer"
	if tenant == "" {
		minimum = "PlatformAdmin"
	}
	if _, err := s.Authorize(ctx, tenant, minimum); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		return nil, domain.Problem("INVALID_ARGUMENT", "audit limit must be 1..100")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,actor_id,tenant_id,action,target_type,target_id,outcome,request_id,occurred_at FROM audit_events WHERE (?='' OR tenant_id=?) ORDER BY occurred_at DESC,rowid DESC LIMIT ?", tenant, tenant, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.AuditEvent{}
	for rows.Next() {
		var e domain.AuditEvent
		if err = rows.Scan(&e.ID, &e.ActorID, &e.TenantID, &e.Action, &e.TargetType, &e.TargetID, &e.Outcome, &e.RequestID, &e.OccurredAt); err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}
func (s *Store) VisibleTenants(ctx context.Context) ([]domain.Tenant, error) {
	u, err := s.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT t.id,t.name,t.slug,t.created_at FROM tenants t WHERE ? OR EXISTS(SELECT 1 FROM tenant_memberships m WHERE m.tenant_id=t.id AND m.user_id=?) ORDER BY t.id LIMIT 100", u.PlatformAdmin, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Tenant{}
	for rows.Next() {
		var t domain.Tenant
		if err = rows.Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}
func (s *Store) VisibleClusters(ctx context.Context) ([]domain.Cluster, error) {
	u, err := s.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	all, err := s.Clusters(ctx)
	if err != nil {
		return nil, err
	}
	if u.PlatformAdmin {
		return all, nil
	}
	items := []domain.Cluster{}
	for _, c := range all {
		var count int
		if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM tenant_profiles p JOIN tenant_memberships m ON m.tenant_id=p.tenant_id WHERE p.cluster_id=? AND m.user_id=?", c.ID, u.ID).Scan(&count); err != nil {
			return nil, err
		}
		if count > 0 {
			items = append(items, c)
		}
	}
	return items, nil
}
func (s *Store) ScopedApps(ctx context.Context, tenant, after string, limit int) ([]domain.Application, error) {
	if _, err := s.Authorize(ctx, tenant, "Viewer"); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		return nil, domain.Problem("INVALID_ARGUMENT", "apps limit must be 1..100")
	}
	return s.Apps(ctx, tenant, after, limit)
}
func (s *Store) ScopedDetail(ctx context.Context, tenant, id string, limit int) (domain.Detail, error) {
	if _, err := s.Authorize(ctx, tenant, "Viewer"); err != nil {
		return domain.Detail{}, err
	}
	if limit < 1 || limit > 100 {
		return domain.Detail{}, domain.Problem("INVALID_ARGUMENT", "detail limit must be 1..100")
	}
	return s.Detail(ctx, tenant, id, limit)
}
func (s *Store) ScopedOperation(ctx context.Context, id string) (domain.Operation, error) {
	o, err := s.Operation(ctx, id)
	if err != nil {
		return o, err
	}
	a, err := s.App(ctx, o.AppID)
	if err != nil {
		return o, err
	}
	if _, err = s.Authorize(ctx, a.TenantID, "Viewer"); err != nil {
		return domain.Operation{}, err
	}
	return o, nil
}
func (s *Store) Profiles(ctx context.Context, tenant string) ([]domain.TenantProfile, error) {
	if _, err := s.Authorize(ctx, tenant, "Viewer"); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT profile_json FROM tenant_profiles WHERE tenant_id=? ORDER BY cluster_id", tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.TenantProfile{}
	for rows.Next() {
		var raw string
		var p domain.TenantProfile
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
func (s *Store) ActivateTenant(ctx context.Context, name, ownerID string, p domain.TenantProfile) error {
	if !domain.ValidDisplayName(name) || ownerID == "" {
		return domain.Problem("INVALID_ARGUMENT", "tenant display name and verified Owner user ID required")
	}
	if err := p.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = authorize(ctx, tx, "", "PlatformAdmin"); err != nil {
		return err
	}
	var connected string
	if err = tx.QueryRowContext(ctx, "SELECT state FROM clusters WHERE id=?", p.ClusterID).Scan(&connected); err != nil {
		return missing(err)
	}
	if connected != "CONNECTED" {
		return domain.Problem("CLUSTER_OFFLINE", "Worker must be connected for tenant activation")
	}
	var users int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE id=? AND email!=''", ownerID).Scan(&users); err != nil {
		return err
	}
	if users != 1 {
		return domain.Problem("INVALID_ARGUMENT", "Owner must have completed a verified OIDC login")
	}
	var existingName string
	err = tx.QueryRowContext(ctx, "SELECT name FROM tenants WHERE id=?", p.TenantID).Scan(&existingName)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err = tx.ExecContext(ctx, "INSERT INTO tenants VALUES(?,?,?,?)", p.TenantID, name, p.TenantID, time.Now().UnixMilli()); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO tenant_memberships VALUES(?,?,'Owner')", p.TenantID, ownerID); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		var role string
		if err = tx.QueryRowContext(ctx, "SELECT role FROM tenant_memberships WHERE tenant_id=? AND user_id=?", p.TenantID, ownerID).Scan(&role); err != nil || role != "Owner" || existingName != name {
			return domain.Problem("CONFLICT", "existing tenant identity or Owner differs")
		}
	}
	var oldUID string
	err = tx.QueryRowContext(ctx, "SELECT bootstrap_uid FROM tenant_profiles WHERE tenant_id=? AND cluster_id=?", p.TenantID, p.ClusterID).Scan(&oldUID)
	if err == nil && oldUID != p.BootstrapUID {
		return domain.Problem("OWNERSHIP_CONFLICT", "Worker bootstrap receipt UID changed; controlled migration required")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO tenant_profiles VALUES(?,?,?,?,?) ON CONFLICT(tenant_id,cluster_id) DO UPDATE SET profile_json=excluded.profile_json", p.TenantID, p.ClusterID, p.Namespace, p.BootstrapUID, string(raw)); err != nil {
		return err
	}
	if err = audit(ctx, tx, p.TenantID, "TENANT_ACTIVATE", "tenant", p.TenantID, "SUCCEEDED"); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Members(ctx context.Context, tenant string) ([]domain.Membership, error) {
	if _, err := s.Authorize(ctx, tenant, "Viewer"); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT u.id,u.email,m.role FROM tenant_memberships m JOIN users u ON u.id=m.user_id WHERE m.tenant_id=? ORDER BY u.id LIMIT 100", tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Membership{}
	for rows.Next() {
		var m domain.Membership
		if err = rows.Scan(&m.UserID, &m.Email, &m.Role); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}
func (s *Store) SetMember(ctx context.Context, tenant, userID, role string) error {
	if userID == "" || (role != "" && !domain.ValidRole(role)) {
		return domain.Problem("INVALID_ARGUMENT", "known user ID and Owner/Developer/Viewer role required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = authorize(ctx, tx, tenant, "Owner"); err != nil {
		return err
	}
	var exists int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE id=? AND email!=''", userID).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return domain.Problem("NOT_FOUND", "user must first complete an OIDC login")
	}
	var oldRole string
	err = tx.QueryRowContext(ctx, "SELECT role FROM tenant_memberships WHERE tenant_id=? AND user_id=?", tenant, userID).Scan(&oldRole)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if oldRole == "Owner" && role != "Owner" {
		var owners int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM tenant_memberships WHERE tenant_id=? AND role='Owner'", tenant).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return domain.Problem("CONFLICT", "cannot remove or demote the last tenant Owner")
		}
	}
	if role == "" {
		_, err = tx.ExecContext(ctx, "DELETE FROM tenant_memberships WHERE tenant_id=? AND user_id=?", tenant, userID)
	} else {
		_, err = tx.ExecContext(ctx, "INSERT INTO tenant_memberships VALUES(?,?,?) ON CONFLICT(tenant_id,user_id) DO UPDATE SET role=excluded.role", tenant, userID, role)
	}
	if err != nil {
		return err
	}
	if err = audit(ctx, tx, tenant, "MEMBER_CHANGE", "user", userID, "SUCCEEDED"); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Users(ctx context.Context) ([]domain.User, error) {
	if _, err := s.Authorize(ctx, "", "PlatformAdmin"); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,issuer,oidc_subject,email,platform_admin,created_at FROM users ORDER BY id LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.User{}
	for rows.Next() {
		var u domain.User
		if err = rows.Scan(&u.ID, &u.Issuer, &u.Subject, &u.Email, &u.PlatformAdmin, &u.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, u)
	}
	return items, rows.Err()
}

func RandomSecret() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic("cryptographic randomness unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
func SecretHash(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

type LoginTransaction struct {
	Nonce, Verifier string
	ExpiresAt       int64
}

func (s *Store) BeginLogin(ctx context.Context, state, binding, nonce, verifier string, expires time.Time) error {
	if !s.secure || len(state) != 43 || len(binding) != 43 || len(nonce) != 43 || len(verifier) != 43 || !expires.After(time.Now()) || expires.After(time.Now().Add(5*time.Minute+time.Second)) {
		return domain.Problem("INVALID_ARGUMENT", "invalid login transaction")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM login_transactions WHERE expires_at<=?", time.Now().UnixMilli()); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM login_transactions").Scan(&count); err != nil {
		return err
	}
	if count >= 256 {
		return domain.Problem("UNAVAILABLE", "too many pending logins")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO login_transactions VALUES(?,?,?,?,?)", SecretHash(state), SecretHash(binding), nonce, verifier, expires.UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ConsumeLogin(ctx context.Context, state, binding string) (LoginTransaction, error) {
	var v LoginTransaction
	if len(state) != 43 {
		return v, domain.Problem("UNAUTHENTICATED", "invalid or expired login transaction")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	var hash string
	err = tx.QueryRowContext(ctx, "SELECT binding_hash,nonce,verifier,expires_at FROM login_transactions WHERE state_hash=?", SecretHash(state)).Scan(&hash, &v.Nonce, &v.Verifier, &v.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return v, domain.Problem("UNAUTHENTICATED", "invalid or reused state")
	}
	if err != nil {
		return v, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM login_transactions WHERE state_hash=?", SecretHash(state)); err != nil {
		return v, err
	}
	if err = tx.Commit(); err != nil {
		return v, err
	}
	// A wrong browser consumes the transaction too; retry requires a fresh login.
	if len(binding) != 43 || v.ExpiresAt <= time.Now().UnixMilli() || subtle.ConstantTimeCompare([]byte(hash), []byte(SecretHash(binding))) != 1 {
		return LoginTransaction{}, domain.Problem("UNAUTHENTICATED", "invalid browser binding or expired state")
	}
	return v, nil
}

type Session struct {
	User       domain.User
	Hash, CSRF string
	ExpiresAt  int64
}

// IssueSession accepts identity only after the caller verifies the ID Token.
// No role, platform-admin, access token or ID Token is accepted or stored.
func (s *Store) IssueSession(ctx context.Context, issuer, subject, email string, expires time.Time) (Session, string, error) {
	var out Session
	if !s.secure || subject == "" || len(subject) > 512 || len(email) > 254 || strings.ContainsAny(email, "\x00\r\n") || !expires.After(time.Now()) || expires.After(time.Now().Add(8*time.Hour+time.Second)) {
		return out, "", domain.Problem("UNAUTHENTICATED", "invalid verified login identity or expiry")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, "", err
	}
	defer tx.Rollback()
	var expected string
	if err = tx.QueryRowContext(ctx, "SELECT issuer FROM security_settings WHERE id=1").Scan(&expected); err != nil {
		return out, "", err
	}
	if issuer != expected {
		return out, "", domain.Problem("UNAUTHENTICATED", "unexpected issuer")
	}
	now := time.Now().UnixMilli()
	if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,issuer,oidc_subject,email,created_at) VALUES(?,?,?,?,?) ON CONFLICT(issuer,oidc_subject) DO UPDATE SET email=excluded.email", uuid.NewString(), issuer, subject, email, now); err != nil {
		return out, "", err
	}
	if err = tx.QueryRowContext(ctx, "SELECT id,issuer,oidc_subject,email,platform_admin,created_at FROM users WHERE issuer=? AND oidc_subject=?", issuer, subject).Scan(&out.User.ID, &out.User.Issuer, &out.User.Subject, &out.User.Email, &out.User.PlatformAdmin, &out.User.CreatedAt); err != nil {
		return out, "", err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<=?", now); err != nil {
		return out, "", err
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM sessions WHERE user_id=?", out.User.ID).Scan(&count); err != nil {
		return out, "", err
	}
	if count >= 20 {
		return out, "", domain.Problem("CONFLICT", "too many active sessions; log out or wait for expiry")
	}
	token := RandomSecret()
	out.Hash = SecretHash(token)
	out.CSRF = RandomSecret()
	out.ExpiresAt = expires.UnixMilli()
	if _, err = tx.ExecContext(ctx, "INSERT INTO sessions VALUES(?,?,?,?,?)", out.Hash, out.User.ID, out.CSRF, out.ExpiresAt, now); err != nil {
		return out, "", err
	}
	loginCtx := domain.WithActor(ctx, domain.Actor{UserID: out.User.ID, SessionHash: out.Hash, RequestID: domain.ActorContext(ctx).RequestID})
	if err = audit(loginCtx, tx, "", "LOGIN", "user", out.User.ID, "SUCCEEDED"); err != nil {
		return out, "", err
	}
	if err = tx.Commit(); err != nil {
		return out, "", err
	}
	return out, token, nil
}
func (s *Store) Session(ctx context.Context, token string) (Session, error) {
	var out Session
	if len(token) != 43 {
		return out, domain.Problem("UNAUTHENTICATED", "valid platform session required")
	}
	out.Hash = SecretHash(token)
	err := s.db.QueryRowContext(ctx, "SELECT u.id,u.issuer,u.oidc_subject,u.email,u.platform_admin,u.created_at,s.csrf,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>?", out.Hash, time.Now().UnixMilli()).Scan(&out.User.ID, &out.User.Issuer, &out.User.Subject, &out.User.Email, &out.User.PlatformAdmin, &out.User.CreatedAt, &out.CSRF, &out.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, domain.Problem("UNAUTHENTICATED", "session expired or revoked")
	}
	return out, err
}
func (s *Store) RevokeSession(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = authorizedUser(ctx, tx); err != nil {
		return err
	}
	if err = audit(ctx, tx, "", "LOGOUT", "session", "", "SUCCEEDED"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=? AND user_id=?", domain.ActorContext(ctx).SessionHash, domain.ActorContext(ctx).UserID); err != nil {
		return err
	}
	return tx.Commit()
}

// Estimate only declared Application requests and rollout surge. Other Pods and
// concurrent Admission are authoritative in Kubernetes, never inferred as success.
func estimateQuota(ctx context.Context, tx *sql.Tx, a domain.Application, spec platform.ApplicationSpec) error {
	var raw string
	if err := tx.QueryRowContext(ctx, "SELECT profile_json FROM tenant_profiles WHERE tenant_id=? AND cluster_id=?", a.TenantID, a.ClusterID).Scan(&raw); err != nil {
		return missing(err)
	}
	var p domain.TenantProfile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return err
	}
	totals := map[string]resource.Quantity{}
	for key := range p.Hard {
		totals[key] = resource.MustParse("0")
	}
	add := func(s platform.ApplicationSpec, n int64) {
		vals := map[string]string{"requests.cpu": s.Resources.Requests.CPU, "requests.memory": s.Resources.Requests.Memory, "limits.cpu": s.Resources.Limits.CPU, "limits.memory": s.Resources.Limits.Memory}
		for key, val := range vals {
			q := resource.MustParse(val)
			q.Mul(n)
			sum := totals[key]
			sum.Add(q)
			totals[key] = sum
		}
		q := totals["pods"]
		q.Add(*resource.NewQuantity(n, resource.DecimalSI))
		totals["pods"] = q
	}
	rows, err := tx.QueryContext(ctx, "SELECT r.spec_snapshot_json FROM applications a JOIN releases r ON r.app_id=a.id AND r.revision=a.desired_revision WHERE a.tenant_id=? AND a.cluster_id=? AND a.id!=? AND a.status NOT IN ('DELETED','DELETING')", a.TenantID, a.ClusterID, a.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var snap string
		var other platform.ApplicationSpec
		if err = rows.Scan(&snap); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal([]byte(snap), &other); err != nil {
			rows.Close()
			return err
		}
		add(other, int64(other.Replicas))
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	add(spec, int64(spec.Replicas)+1)
	for key, sum := range totals {
		hard := resource.MustParse(p.Hard[key])
		if sum.Cmp(hard) > 0 {
			return domain.Problem("QUOTA_EXCEEDED", "declared applications plus one rollout surge exceed "+key+"; Kubernetes Pod Admission remains authoritative")
		}
	}
	return nil
}
