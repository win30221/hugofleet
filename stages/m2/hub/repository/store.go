package repository

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m5/observability"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

//go:embed 001_initial.sql
var migration string

//go:embed 002_observations.sql
var observationMigration string

//go:embed 003_agent.sql
var agentMigration string

//go:embed 004_security.sql
var securityMigration string

//go:embed 005_observability.sql
var observabilityMigration string

type Store struct {
	// Terminal runs only after an actual terminal transition commits. Set before
	// starting the Runner; instrumentation cannot change transaction outcomes.
	Terminal func(domain.Operation, domain.Application)
	db       *sql.DB
	lock     *os.File
	secure   bool
}

func Open(ctx context.Context, path string) (*Store, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, err
	}
	lock, err := lockDatabase(absolute + ".lock")
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(FULL)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		lock.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db, lock: lock}
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()
	var mode string
	if err = db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		return nil, err
	}
	if mode != "wal" {
		return nil, fmt.Errorf("SQLite requires WAL")
	}
	if err = os.Chmod(absolute, 0600); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return nil, err
	}
	if version > 5 {
		return nil, fmt.Errorf("database schema is newer than this Hub")
	}
	if version == 0 {
		if _, err = tx.ExecContext(ctx, migration); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
			return nil, err
		}
	}
	if version < 2 {
		if _, err = tx.ExecContext(ctx, observationMigration); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "PRAGMA user_version=2"); err != nil {
			return nil, err
		}
	}
	if version < 3 {
		if _, err = tx.ExecContext(ctx, agentMigration); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "PRAGMA user_version=3"); err != nil {
			return nil, err
		}
	}
	if version < 4 {
		if _, err = tx.ExecContext(ctx, securityMigration); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "PRAGMA user_version=4"); err != nil {
			return nil, err
		}
	}
	if version < 5 {
		if _, err = tx.ExecContext(ctx, observabilityMigration); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "PRAGMA user_version=5"); err != nil {
			return nil, err
		}
	}
	var securityCount int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM security_settings").Scan(&securityCount); err != nil {
		return nil, err
	}
	s.secure = securityCount == 1
	now := time.Now().UnixMilli()
	if !s.secure {
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO tenants VALUES(?,?,?,?)", domain.TenantID, "Local development", "demo", now); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO clusters(id,name,state,last_seen_at,registered_at) VALUES(?,?,?,?,?)", domain.ClusterID, "Local kind (development only)", "DISCONNECTED", 0, now); err != nil {
			return nil, err
		}
	}
	// A new process has not yet confirmed connectivity; never report stale CONNECTED.
	if _, err = tx.ExecContext(ctx, "UPDATE clusters SET state='DISCONNECTED'"); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	ok = true
	return s, nil
}
func (s *Store) Close() error                   { return errors.Join(s.db.Close(), s.lock.Close()) }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

type scanner interface{ Scan(...any) error }
type querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

const appColumns = "id,tenant_id,cluster_id,name,desired_revision,applied_revision,last_successful_revision,status,last_observed_at,kubernetes_uid,observation_error_code,observation_error_message,observed_snapshot"
const opColumns = "id,app_id,COALESCE(release_id,''),command_id,type,state,error_code,error_message,created_at,deadline_at,finished_at,request_id,traceparent,tracestate"
const releaseColumns = "id,app_id,revision,spec_snapshot_json,state,created_by,created_at,rollback_source_revision"

func appScan(row scanner) (a domain.Application, err error) {
	var snapshot string
	err = row.Scan(&a.ID, &a.TenantID, &a.ClusterID, &a.Name, &a.DesiredRevision, &a.AppliedRevision, &a.LastSuccessfulRevision, &a.Status, &a.LastObservedAt, &a.KubernetesUID, &a.ObservationErrorCode, &a.ObservationErrorMessage, &snapshot)
	if err == nil {
		err = json.Unmarshal([]byte(snapshot), &a.Observed)
	}
	return
}
func opScan(row scanner) (o domain.Operation, err error) {
	err = row.Scan(&o.ID, &o.AppID, &o.ReleaseID, &o.CommandID, &o.Type, &o.State, &o.ErrorCode, &o.ErrorMessage, &o.CreatedAt, &o.DeadlineAt, &o.FinishedAt, &o.RequestID, &o.Traceparent, &o.Tracestate)
	return
}
func releaseScan(row scanner) (r domain.Release, err error) {
	var raw string
	err = row.Scan(&r.ID, &r.AppID, &r.Revision, &raw, &r.State, &r.CreatedBy, &r.CreatedAt, &r.RollbackSourceRevision)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &r.Spec)
	}
	return
}
func missing(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Problem("NOT_FOUND", "resource not found")
	}
	return err
}
func getApp(ctx context.Context, q querier, id string) (domain.Application, error) {
	a, e := appScan(q.QueryRowContext(ctx, "SELECT "+appColumns+" FROM applications WHERE id=?", id))
	return a, missing(e)
}
func (s *Store) App(ctx context.Context, id string) (domain.Application, error) {
	return getApp(ctx, s.db, id)
}
func (s *Store) Operation(ctx context.Context, id string) (domain.Operation, error) {
	o, e := opScan(s.db.QueryRowContext(ctx, "SELECT "+opColumns+" FROM operations WHERE id=?", id))
	return o, missing(e)
}
func getRelease(ctx context.Context, q querier, app string, rev int64) (domain.Release, error) {
	r, e := releaseScan(q.QueryRowContext(ctx, "SELECT "+releaseColumns+" FROM releases WHERE app_id=? AND revision=?", app, rev))
	return r, missing(e)
}
func (s *Store) Release(ctx context.Context, app string, rev int64) (domain.Release, error) {
	return getRelease(ctx, s.db, app, rev)
}
func (s *Store) Tenants(ctx context.Context) ([]domain.Tenant, error) {
	var t domain.Tenant
	e := s.db.QueryRowContext(ctx, "SELECT id,name,slug,created_at FROM tenants WHERE id=?", domain.TenantID).Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt)
	return []domain.Tenant{t}, e
}
func (s *Store) Clusters(ctx context.Context) ([]domain.Cluster, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,state,last_seen_at,registered_at,agent_version,last_error FROM clusters ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Cluster{}
	for rows.Next() {
		var c domain.Cluster
		if err = rows.Scan(&c.ID, &c.Name, &c.State, &c.LastSeenAt, &c.RegisteredAt, &c.AgentVersion, &c.LastError); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
func (s *Store) Connectivity(ctx context.Context, connected bool) error {
	state := "DISCONNECTED"
	if connected {
		state = "CONNECTED"
	}
	_, err := s.db.ExecContext(ctx, "UPDATE clusters SET state=?,last_seen_at=CASE WHEN ? THEN ? ELSE last_seen_at END WHERE id=?", state, connected, time.Now().UnixMilli(), domain.ClusterID)
	return err
}
func (s *Store) Apps(ctx context.Context, tenant, after string, limit int) ([]domain.Application, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+appColumns+" FROM applications WHERE tenant_id=? AND id>? ORDER BY id LIMIT ?", tenant, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Application{}
	for rows.Next() {
		a, e := appScan(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
func (s *Store) Pending(ctx context.Context, limit int) ([]domain.Operation, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT "+opColumns+" FROM operations WHERE state IN ('QUEUED','DISPATCHED','RUNNING') ORDER BY created_at,id LIMIT ?", limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ops := []domain.Operation{}
	for rows.Next() {
		o, e := opScan(rows)
		if e != nil {
			return nil, e
		}
		ops = append(ops, o)
	}
	return ops, rows.Err()
}

// Only live applications need observations. Deleted history must not delay the
// bounded reconciliation of the at most 30 live applications.
func (s *Store) ObservationApps(ctx context.Context) ([]domain.Application, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+appColumns+" FROM applications WHERE status NOT IN ('DELETED','DELETING') ORDER BY id LIMIT 30")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Application{}
	for rows.Next() {
		a, e := appScan(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
func addEvent(ctx context.Context, tx *sql.Tx, op, kind, message string) error {
	_, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO operation_events VALUES(?,?,?,?,?)", uuid.NewString(), op, kind, message, time.Now().UnixMilli())
	return err
}

// Accept commits intent, immutable snapshot, operation and deduplication together.
// The Kubernetes side effect happens only after this transaction has committed.
func (s *Store) Accept(ctx context.Context, c domain.Change, timeout time.Duration) (out domain.Accepted, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var hash, opID string
	if s.secure {
		minimum := "Developer"
		if c.Kind == "DELETE" {
			minimum = "Owner"
		}
		if _, err = authorize(ctx, tx, c.TenantID, minimum); err != nil {
			return out, err
		}
		if c.AppID != "" {
			a, e := getApp(ctx, tx, c.AppID)
			if e != nil {
				return out, e
			}
			if a.TenantID != c.TenantID {
				return out, domain.Problem("NOT_FOUND", "application not found")
			}
		}
	}
	err = tx.QueryRowContext(ctx, "SELECT request_hash,operation_id FROM idempotency_keys WHERE tenant_id=? AND key=?", c.TenantID, c.Key).Scan(&hash, &opID)
	if err == nil {
		if hash != c.Hash {
			return out, domain.Problem("CONFLICT", "Idempotency-Key was already used with a different request")
		}
		o, e := opScan(tx.QueryRowContext(ctx, "SELECT "+opColumns+" FROM operations WHERE id=?", opID))
		if e != nil {
			return out, e
		}
		var rev int64
		if o.ReleaseID != "" {
			if e = tx.QueryRowContext(ctx, "SELECT revision FROM releases WHERE id=?", o.ReleaseID).Scan(&rev); e != nil {
				return out, e
			}
		} else {
			if e = tx.QueryRowContext(ctx, "SELECT desired_revision FROM applications WHERE id=?", o.AppID).Scan(&rev); e != nil {
				return out, e
			}
		}
		if s.secure {
			if e = audit(ctx, tx, c.TenantID, c.Kind, "application", o.AppID, "REPLAY"); e != nil {
				return out, e
			}
			if e = tx.Commit(); e != nil {
				return out, e
			}
		}
		return domain.Accepted{ApplicationID: o.AppID, OperationID: o.ID, Revision: rev, State: o.State}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	var tenantExists int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM tenants WHERE id=?", c.TenantID).Scan(&tenantExists); err != nil {
		return out, err
	}
	if tenantExists != 1 || (!s.secure && c.TenantID != domain.TenantID) {
		return out, domain.Problem("NOT_FOUND", "tenant not found")
	}
	var a domain.Application
	if c.Kind == "CREATE" {
		if c.Expected != 0 {
			return out, domain.Problem("CONFLICT", "initial expected_desired_revision must be zero")
		}
		var state string
		if err = tx.QueryRowContext(ctx, "SELECT state FROM clusters WHERE id=?", c.ClusterID).Scan(&state); err != nil {
			return out, missing(err)
		}
		if state != "CONNECTED" {
			return out, domain.Problem("CLUSTER_OFFLINE", "local cluster is not connected")
		}
		if s.secure {
			var approved int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM tenant_profiles WHERE tenant_id=? AND cluster_id=?", c.TenantID, c.ClusterID).Scan(&approved); err != nil {
				return out, err
			}
			if approved != 1 {
				return out, domain.Problem("PERMISSION_DENIED", "tenant is not provisioned on this cluster")
			}
		}
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM applications WHERE tenant_id=? AND name=?", c.TenantID, c.Name).Scan(&count); err != nil {
			return out, err
		}
		if count != 0 {
			return out, domain.Problem("CONFLICT", "application name already exists, including retained deletion history")
		}
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM applications WHERE status!='DELETED'").Scan(&count); err != nil {
			return out, err
		}
		if count >= 30 {
			return out, domain.Problem("QUOTA_EXCEEDED", "development Hub supports at most 30 live applications")
		}
		a = domain.Application{ID: uuid.NewString(), TenantID: c.TenantID, ClusterID: c.ClusterID, Name: c.Name, Status: "PENDING"}
		if _, err = tx.ExecContext(ctx, "INSERT INTO applications(id,tenant_id,cluster_id,name,status) VALUES(?,?,?,?,?)", a.ID, a.TenantID, a.ClusterID, a.Name, a.Status); err != nil {
			return out, err
		}
	} else {
		a, err = getApp(ctx, tx, c.AppID)
		if err != nil {
			return out, err
		}
		if a.TenantID != c.TenantID {
			return out, domain.Problem("NOT_FOUND", "application not found")
		}
		if a.Status == "DELETED" || a.Status == "DELETING" {
			return out, domain.Problem("CONFLICT", "application is deleting or deleted")
		}
		if a.DesiredRevision != c.Expected {
			return out, domain.Problem("CONFLICT", "expected_desired_revision does not match current desired revision")
		}
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM operations WHERE app_id=? AND state IN ('QUEUED','DISPATCHED','RUNNING')", a.ID).Scan(&count); err != nil {
			return out, err
		}
		if count != 0 {
			return out, domain.Problem("CONFLICT", "another operation is active for this application")
		}
		var state string
		if err = tx.QueryRowContext(ctx, "SELECT state FROM clusters WHERE id=?", a.ClusterID).Scan(&state); err != nil {
			return out, err
		}
		if state != "CONNECTED" {
			return out, domain.Problem("CLUSTER_OFFLINE", "local cluster is not connected")
		}
	}
	if c.Kind == "ROLLBACK" {
		r, e := getRelease(ctx, tx, a.ID, c.SourceRevision)
		if e != nil {
			return out, e
		}
		if r.State != "ACTIVE" && r.State != "SUPERSEDED" {
			return out, domain.Problem("CONFLICT", "rollback source must have completed a successful rollout")
		}
		c.Spec = r.Spec
	}
	if c.Kind != "DELETE" && c.ValidateSpec != nil {
		if err = c.ValidateSpec(c.Spec); err != nil {
			return out, err
		}
	}
	if s.secure && c.Kind != "DELETE" {
		if err = estimateQuota(ctx, tx, a, c.Spec); err != nil {
			return out, err
		}
	}
	now := time.Now().UnixMilli()
	o := domain.Operation{ID: uuid.NewString(), AppID: a.ID, CommandID: uuid.NewString(), Type: c.Kind, State: "QUEUED", CreatedAt: now, DeadlineAt: time.Now().Add(timeout).UnixMilli()}
	o.Traceparent, o.Tracestate = observability.Inject(ctx)
	actor := domain.ActorContext(ctx)
	if s.secure {
		o.RequestID = actor.RequestID
	}
	rev := a.DesiredRevision
	status := "DELETING"
	if c.Kind != "DELETE" {
		rev++
		c.Spec.Revision = rev
		raw, e := json.Marshal(c.Spec)
		if e != nil {
			return out, e
		}
		o.ReleaseID = uuid.NewString()
		status = "DEPLOYING"
		createdBy := "local-development-admin"
		if s.secure {
			createdBy = actor.UserID
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO releases VALUES(?,?,?,?,?,?,?,?)", o.ReleaseID, a.ID, rev, string(raw), "PENDING", createdBy, now, c.SourceRevision); err != nil {
			return out, err
		}
	}
	result, e := tx.ExecContext(ctx, "UPDATE applications SET desired_revision=?,status=? WHERE id=? AND desired_revision=?", rev, status, a.ID, a.DesiredRevision)
	if e != nil {
		return out, e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return out, e
	}
	if n != 1 {
		return out, domain.Problem("CONFLICT", "application changed concurrently")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO operations(id,app_id,release_id,command_id,type,state,created_at,deadline_at,request_id,traceparent,tracestate) VALUES(?,?,NULLIF(?,''),?,?,?,?,?,?,?,?)", o.ID, a.ID, o.ReleaseID, o.CommandID, o.Type, o.State, o.CreatedAt, o.DeadlineAt, o.RequestID, o.Traceparent, o.Tracestate); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO idempotency_keys VALUES(?,?,?,?)", c.TenantID, c.Key, c.Hash, o.ID); err != nil {
		return out, err
	}
	if err = addEvent(ctx, tx, o.ID, "QUEUED", "Intent durably accepted"); err != nil {
		return out, err
	}
	if s.secure {
		if err = audit(ctx, tx, c.TenantID, c.Kind, "application", a.ID, "ACCEPTED"); err != nil {
			return out, err
		}
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	return domain.Accepted{ApplicationID: a.ID, OperationID: o.ID, Revision: rev, State: "QUEUED"}, nil
}
func (s *Store) Dispatch(ctx context.Context, op string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE operations SET state='DISPATCHED' WHERE id=? AND state='QUEUED'", op)
	return err
}

// BindDeletionUID preserves a recovered CR identity before deletion can begin.
// It does not claim that the application's desired revision was applied.
func (s *Store) BindDeletionUID(ctx context.Context, o domain.Operation, a domain.Application, uid string) error {
	if uid == "" {
		return domain.Problem("CONFLICT", "cannot bind an empty Kubernetes UID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE applications SET kubernetes_uid=? WHERE id=? AND desired_revision=? AND (kubernetes_uid='' OR kubernetes_uid=?) AND EXISTS(SELECT 1 FROM operations WHERE id=? AND app_id=applications.id AND type='DELETE' AND state IN ('DISPATCHED','RUNNING'))", uid, a.ID, a.DesiredRevision, uid, o.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.Problem("CONFLICT", "deletion UID binding does not match the active operation")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE operations SET state='RUNNING' WHERE id=?", o.ID); err != nil {
		return err
	}
	if err = addEvent(ctx, tx, o.ID, "UID_BOUND", "Recovered Kubernetes UID persisted before deletion"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Applied(ctx context.Context, o domain.Operation, a domain.Application, uid string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE operations SET state='RUNNING' WHERE id=? AND state IN ('DISPATCHED','RUNNING')", o.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE applications SET applied_revision=?,kubernetes_uid=? WHERE id=? AND desired_revision=? AND (kubernetes_uid='' OR kubernetes_uid=?)", a.DesiredRevision, uid, a.ID, a.DesiredRevision, uid); err != nil {
		return err
	}
	if err = addEvent(ctx, tx, o.ID, "APPLIED", "Application CR write confirmed; rollout is still pending"); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Finish(ctx context.Context, o domain.Operation, a domain.Application, state, code, message string, observed ...domain.Observation) error {
	if len(observed) > 1 {
		return domain.Problem("INVALID_ARGUMENT", "one terminal observation at most")
	}
	finished := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE operations SET state=?,error_code=?,error_message=?,finished_at=? WHERE id=? AND state IN ('QUEUED','DISPATCHED','RUNNING')", state, code, message, finished, o.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	if len(observed) == 1 {
		ob := observed[0]
		current, e := getApp(ctx, tx, a.ID)
		if e != nil {
			return e
		}
		if (ob.UID != "" && current.KubernetesUID != "" && ob.UID != current.KubernetesUID) || (ob.Applied && ob.UID == "") {
			return domain.Problem("OWNERSHIP_CONFLICT", "terminal observation does not match the bound Worker UID")
		}
		raw, e := json.Marshal(ob)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE applications SET observed_snapshot=?,last_observed_at=?,observation_error_code=?,observation_error_message=? WHERE id=? AND desired_revision=?", string(raw), finished, ob.ErrorCode, ob.Message, a.ID, a.DesiredRevision); e != nil {
			return e
		}
	}
	status := "DEGRADED"
	if state == "SUCCEEDED" {
		status = "READY"
		if o.Type == "DELETE" {
			status = "DELETED"
		}
	}
	if o.ReleaseID != "" {
		releaseState := "FAILED"
		if state == "SUCCEEDED" {
			releaseState = "ACTIVE"
			if _, err = tx.ExecContext(ctx, "UPDATE releases SET state='SUPERSEDED' WHERE app_id=? AND state='ACTIVE'", a.ID); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE releases SET state=? WHERE id=?", releaseState, o.ReleaseID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE applications SET status=?,last_successful_revision=CASE WHEN ?='SUCCEEDED' AND ?!='DELETE' THEN ? ELSE last_successful_revision END WHERE id=? AND desired_revision=?", status, state, o.Type, a.DesiredRevision, a.ID, a.DesiredRevision); err != nil {
		return err
	}
	if err = addEvent(ctx, tx, o.ID, state, message); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if s.Terminal != nil {
		o.State = state
		o.ErrorCode = code
		o.FinishedAt = finished
		s.Terminal(o, a)
	}
	return nil
}
func (s *Store) Observe(ctx context.Context, a domain.Application, ob domain.Observation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := getApp(ctx, tx, a.ID)
	if err != nil {
		return err
	}
	// An observation read before a new intent or deletion must not affect it.
	if current.DesiredRevision != a.DesiredRevision || current.Status == "DELETING" || current.Status == "DELETED" {
		return nil
	}
	if ob.UID != "" && current.KubernetesUID != "" && current.KubernetesUID != ob.UID {
		return domain.Problem("OWNERSHIP_CONFLICT", "observation does not match the persisted Kubernetes UID")
	}
	if ob.Applied && ob.UID == "" {
		return domain.Problem("CONFLICT", "applied observation requires a Kubernetes UID")
	}
	status := "DEPLOYING"
	if ob.Ready {
		status = "READY"
	} else if ob.ErrorCode != "" {
		status = "DEGRADED"
	}
	applied, uid := current.AppliedRevision, current.KubernetesUID
	if ob.Applied {
		applied, uid = a.DesiredRevision, ob.UID
	}
	snapshot, e := json.Marshal(ob)
	if e != nil {
		return e
	}
	if _, err = tx.ExecContext(ctx, "UPDATE applications SET status=?,last_observed_at=?,observation_error_code=?,observation_error_message=?,applied_revision=?,kubernetes_uid=?,observed_snapshot=? WHERE id=? AND desired_revision=? AND status NOT IN ('DELETING','DELETED')", status, time.Now().UnixMilli(), ob.ErrorCode, ob.Message, applied, uid, string(snapshot), a.ID, a.DesiredRevision); err != nil {
		return err
	}
	var id, state string
	err = tx.QueryRowContext(ctx, "SELECT o.id,o.state FROM operations o JOIN releases r ON o.release_id=r.id WHERE r.app_id=? AND r.revision=?", a.ID, a.DesiredRevision).Scan(&id, &state)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if id != "" {
		if ob.Applied && current.AppliedRevision != applied {
			if err = addEvent(ctx, tx, id, "APPLIED", "Bound Application CR and exact revision snapshot verified by observation"); err != nil {
				return err
			}
		}
		if ob.ErrorCode != "" && (current.ObservationErrorCode != ob.ErrorCode || current.ObservationErrorMessage != ob.Message) {
			if err = addEvent(ctx, tx, id, "OBSERVED_DEGRADED", ob.ErrorCode+": "+ob.Message); err != nil {
				return err
			}
		} else if ob.ErrorCode == "" && current.ObservationErrorCode != "" {
			if err = addEvent(ctx, tx, id, "OBSERVED_RECOVERED", "Current observation no longer reports the previous failure; state="+status); err != nil {
				return err
			}
		}
		if ob.Ready && (state == "TIMED_OUT" || state == "FAILED") {
			if err = addEvent(ctx, tx, id, "LATE_READY", "Current revision became Ready after its operation terminated; failed release history is preserved"); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
func (s *Store) Unknown(ctx context.Context, a domain.Application) error {
	_, err := s.db.ExecContext(ctx, "UPDATE applications SET status='UNKNOWN' WHERE id=? AND desired_revision=? AND status NOT IN ('DELETING','DELETED') AND last_observed_at<?", a.ID, a.DesiredRevision, time.Now().Add(-15*time.Second).UnixMilli())
	return err
}
func (s *Store) Detail(ctx context.Context, tenant, id string, limit int) (d domain.Detail, err error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return d, err
	}
	defer tx.Rollback()
	d.Application, err = getApp(ctx, tx, id)
	if err != nil {
		return d, err
	}
	if d.Application.TenantID != tenant {
		return d, domain.Problem("NOT_FOUND", "application not found")
	}
	d.Releases = []domain.Release{}
	d.Operations = []domain.Operation{}
	d.Events = []domain.Event{}
	rows, err := tx.QueryContext(ctx, "SELECT "+releaseColumns+" FROM releases WHERE app_id=? ORDER BY revision DESC LIMIT ?", id, limit)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		r, e := releaseScan(rows)
		if e != nil {
			rows.Close()
			return d, e
		}
		d.Releases = append(d.Releases, r)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return d, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT "+opColumns+" FROM operations WHERE app_id=? ORDER BY created_at DESC,rowid DESC LIMIT ?", id, limit)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		o, e := opScan(rows)
		if e != nil {
			rows.Close()
			return d, e
		}
		d.Operations = append(d.Operations, o)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return d, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT e.id,e.operation_id,e.kind,e.message,e.occurred_at FROM operation_events e JOIN operations o ON e.operation_id=o.id WHERE o.app_id=? ORDER BY e.occurred_at DESC,e.rowid DESC LIMIT ?", id, limit)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var e domain.Event
		if err = rows.Scan(&e.ID, &e.OperationID, &e.Kind, &e.Message, &e.OccurredAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Events = append(d.Events, e)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return d, err
	}
	return d, tx.Commit()
}
