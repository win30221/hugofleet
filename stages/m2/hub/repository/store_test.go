package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hub.db")
	s, e := Open(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if e = s.Connectivity(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	return s, path
}
func intent(name string) domain.Change {
	spec, _ := domain.DefaultPolicy().Spec(domain.DeployRequest{Image: "pricing-api:v1"})
	return domain.Change{TenantID: domain.TenantID, ClusterID: domain.ClusterID, Name: name, Kind: "CREATE", Key: name, Hash: name, Spec: spec}
}
func accept(t *testing.T, s *Store, c domain.Change) domain.Accepted {
	t.Helper()
	a, e := s.Accept(context.Background(), c, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func finish(t *testing.T, s *Store, o domain.Accepted, state string) {
	t.Helper()
	ctx := context.Background()
	op, e := s.Operation(ctx, o.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	app, e := s.App(ctx, o.ApplicationID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Dispatch(ctx, op.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Applied(ctx, op, app, "k8s-uid"); e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(ctx, op, app, state, "test_reason", "state-machine integration test"); e != nil {
		t.Fatal(e)
	}
}
func code(t *testing.T, e error, want string) {
	t.Helper()
	var p *domain.Error
	if !errors.As(e, &p) || p.Code != want {
		t.Fatalf("error=%v want=%s", e, want)
	}
}
func TestAtomicIdempotencyAndConcurrency(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := intent("pricing-api")
	first := accept(t, s, c)
	again := accept(t, s, c)
	if first != again {
		t.Fatalf("dedup changed operation: %+v %+v", first, again)
	}
	altered := c
	altered.Hash = "different"
	_, e := s.Accept(ctx, altered, time.Minute)
	code(t, e, "CONFLICT")
	d, e := s.Detail(ctx, domain.TenantID, first.ApplicationID, 100)
	if e != nil || len(d.Releases) != 1 || len(d.Operations) != 1 {
		t.Fatalf("duplicate rows: %+v %v", d, e)
	}
	update := c
	update.Kind = "RELEASE"
	update.AppID = first.ApplicationID
	update.Expected = 1
	update.Key = "busy"
	update.Hash = "busy"
	_, e = s.Accept(ctx, update, time.Minute)
	code(t, e, "CONFLICT")
	finish(t, s, first, "SUCCEEDED")
	var wg sync.WaitGroup
	results := make(chan error, 16)
	success := make(chan domain.Accepted, 16)
	start := make(chan struct{})
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			v := update
			v.Key = time.Unix(int64(i), 0).String()
			v.Hash = v.Key
			o, e := s.Accept(ctx, v, time.Minute)
			results <- e
			if e == nil {
				success <- o
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	close(success)
	count := 0
	for e := range results {
		if e == nil {
			count++
		} else {
			code(t, e, "CONFLICT")
		}
	}
	if count != 1 {
		t.Fatalf("%d winners", count)
	}
	d, e = s.Detail(ctx, domain.TenantID, first.ApplicationID, 100)
	if e != nil || len(d.Releases) != 2 || len(d.Operations) != 2 || d.Application.DesiredRevision != 2 || d.Application.LastSuccessfulRevision != 1 {
		t.Fatalf("bad concurrent outcome: %+v %v", d, e)
	}
	winner := <-success
	op, e := s.Operation(ctx, winner.OperationID)
	if e != nil || !op.Active() {
		t.Fatalf("winner not durably active %v", e)
	}
	_, e = s.Detail(ctx, "foreign-tenant", first.ApplicationID, 100)
	code(t, e, "NOT_FOUND")
}

func TestObservationFaultSurvivesRestartWithoutRewritingSuccess(t *testing.T) {
	s, path := testStore(t)
	ctx := context.Background()
	created := accept(t, s, intent("observation-fault"))
	finish(t, s, created, "SUCCEEDED")
	a, err := s.App(ctx, created.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err = s.Observe(ctx, a, domain.Observation{ErrorCode: "APPLICATION_MISSING", Message: "Application CR is missing"}); err != nil {
			t.Fatal(err)
		}
	}
	d, err := s.Detail(ctx, domain.TenantID, a.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	faults := 0
	for _, event := range d.Events {
		if event.Kind == "OBSERVED_DEGRADED" && event.Message == "APPLICATION_MISSING: Application CR is missing" {
			faults++
		}
	}
	if faults != 1 {
		t.Fatalf("observation reason missing or repeated every tick: %+v", d.Events)
	}
	if d.Operations[0].State != "SUCCEEDED" || d.Releases[0].State != "ACTIVE" || d.Application.LastSuccessfulRevision != 1 {
		t.Fatalf("observation corrupted successful history: %+v", d)
	}
	if d.Application.ObservationErrorCode != "APPLICATION_MISSING" || d.Application.ObservationErrorMessage != "Application CR is missing" {
		t.Fatalf("current failure missing: %+v", d.Application)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d, err = s.Detail(ctx, domain.TenantID, a.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if d.Application.Status != "DEGRADED" || len(d.Events) < 1 {
		t.Fatalf("observation was not durable: %+v", d)
	}
	if d.Application.ObservationErrorCode != "APPLICATION_MISSING" {
		t.Fatalf("failure reason lost on restart: %+v", d.Application)
	}
	if err = s.Observe(ctx, d.Application, domain.Observation{Ready: true}); err != nil {
		t.Fatal(err)
	}
	d, err = s.Detail(ctx, domain.TenantID, a.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if d.Application.ObservationErrorCode != "" || d.Application.ObservationErrorMessage != "" {
		t.Fatalf("recovered failure reason not cleared: %+v", d.Application)
	}
	if err = s.Observe(ctx, d.Application, domain.Observation{ErrorCode: "SPEC_DRIFT", Message: "CR differs from accepted intent"}); err != nil {
		t.Fatal(err)
	}
	d, err = s.Detail(ctx, domain.TenantID, a.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	faults = 0
	recoveries := 0
	for _, event := range d.Events {
		if event.Kind == "OBSERVED_DEGRADED" {
			faults++
		}
		if event.Kind == "OBSERVED_RECOVERED" {
			recoveries++
		}
	}
	if faults != 2 || recoveries != 1 {
		t.Fatalf("later faults or recovery were lost: %+v", d.Events)
	}
}

func TestDeletionRecoversUIDWithoutClaimingAppliedRevision(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	created := accept(t, s, intent("uid-recovery"))
	o, _ := s.Operation(ctx, created.OperationID)
	a, _ := s.App(ctx, created.ApplicationID)
	if err := s.Finish(ctx, o, a, "TIMED_OUT", "DEADLINE_EXCEEDED", "create may have reached Kubernetes"); err != nil {
		t.Fatal(err)
	}
	deleted := accept(t, s, domain.Change{TenantID: domain.TenantID, AppID: a.ID, Kind: "DELETE", Key: "delete-recovery", Hash: "delete-recovery", Expected: 1})
	o, _ = s.Operation(ctx, deleted.OperationID)
	a, _ = s.App(ctx, a.ID)
	if err := s.Dispatch(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.BindDeletionUID(ctx, o, a, "recovered-cr-uid"); err != nil {
		t.Fatal(err)
	}
	if err := s.BindDeletionUID(ctx, o, a, "other-cr-uid"); err == nil {
		t.Fatal("deletion rebound a different UID")
	}
	d, err := s.Detail(ctx, domain.TenantID, a.ID, 100)
	if err != nil || d.Application.KubernetesUID != "recovered-cr-uid" || d.Application.AppliedRevision != 0 || d.Application.LastSuccessfulRevision != 0 || d.Operations[0].State != "RUNNING" {
		t.Fatalf("UID recovery changed deployment history: %+v %v", d, err)
	}
	if err := s.Finish(ctx, o, d.Application, "SUCCEEDED", "", "GC confirmed"); err != nil {
		t.Fatal(err)
	}
	if err := s.BindDeletionUID(ctx, o, a, "recovered-cr-uid"); err == nil {
		t.Fatal("terminal deletion accepted a new binding")
	}
}
func TestImmutableRollbackAndTerminalLateObservation(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := intent("rollback")
	one := accept(t, s, c)
	finish(t, s, one, "SUCCEEDED")
	update := c
	update.Kind = "RELEASE"
	update.AppID = one.ApplicationID
	update.Expected = 1
	update.Key = "two"
	update.Hash = "two"
	update.Spec.Image = "pricing-api:v2-broken"
	two := accept(t, s, update)
	finish(t, s, two, "TIMED_OUT")
	a, e := s.App(ctx, one.ApplicationID)
	if e != nil {
		t.Fatal(e)
	}
	if a.DesiredRevision != 2 || a.AppliedRevision != 2 || a.LastSuccessfulRevision != 1 {
		t.Fatalf("revision truth %+v", a)
	}
	if e = s.Observe(ctx, a, domain.Observation{UID: "k8s-uid", Ready: true}); e != nil {
		t.Fatal(e)
	}
	op, e := s.Operation(ctx, two.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(ctx, op, a, "SUCCEEDED", "", "late success"); e != nil {
		t.Fatal(e)
	}
	d, e := s.Detail(ctx, domain.TenantID, a.ID, 100)
	if e != nil || d.Releases[0].State != "FAILED" || d.Application.LastSuccessfulRevision != 1 || d.Application.Status != "READY" {
		t.Fatalf("late success corrupted history %+v %v", d, e)
	}
	op, e = s.Operation(ctx, two.OperationID)
	if e != nil || op.State != "TIMED_OUT" {
		t.Fatalf("terminal op mutated %+v %v", op, e)
	}
	late := 0
	for _, v := range d.Events {
		if v.Kind == "LATE_READY" {
			late++
		}
	}
	if late != 1 {
		t.Fatalf("late event missing: %+v", d.Events)
	}
	rollback := update
	rollback.Kind = "ROLLBACK"
	rollback.Expected = 2
	rollback.SourceRevision = 2
	rollback.Key = "invalid-source"
	rollback.Hash = rollback.Key
	_, e = s.Accept(ctx, rollback, time.Minute)
	code(t, e, "CONFLICT")
	rollback.SourceRevision = 1
	rollback.Key = "three"
	rollback.Hash = "three"
	three := accept(t, s, rollback)
	if three.Revision != 3 {
		t.Fatal("rollback reused revision")
	}
	finish(t, s, three, "SUCCEEDED")
	d, e = s.Detail(ctx, domain.TenantID, a.ID, 100)
	if e != nil || len(d.Releases) != 3 || d.Releases[0].Spec.Image != "pricing-api:v1" || d.Releases[0].RollbackSourceRevision != 1 || d.Releases[1].State != "FAILED" || d.Releases[2].State != "SUPERSEDED" || d.Application.LastSuccessfulRevision != 3 {
		t.Fatalf("bad rollback %+v %v", d, e)
	}
	if _, e = s.db.ExecContext(ctx, "UPDATE releases SET spec_snapshot_json='{}' WHERE id=?", d.Releases[2].ID); e == nil {
		t.Fatal("snapshot mutation accepted")
	}
	if _, e = s.db.ExecContext(ctx, "UPDATE operations SET state='SUCCEEDED' WHERE id=?", two.OperationID); e == nil {
		t.Fatal("terminal state mutation accepted")
	}
	if _, e = s.db.ExecContext(ctx, "UPDATE releases SET state='ACTIVE' WHERE id=?", d.Releases[1].ID); e == nil {
		t.Fatal("failed release mutation accepted")
	}
}
func TestWALMigrationLockAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "restart.db")
	s, e := Open(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	var mode string
	var version, fk int
	if e = s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); e != nil {
		t.Fatal(e)
	}
	s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk)
	if mode != "wal" || version != 5 || fk != 1 {
		t.Fatalf("bad DB configuration %s %d %d", mode, version, fk)
	}
	if other, e := Open(ctx, path); e == nil {
		other.Close()
		t.Fatal("second Hub opened same database")
	}
	if e = s.Connectivity(ctx, true); e != nil {
		t.Fatal(e)
	}
	first := accept(t, s, intent("restart"))
	op, e := s.Operation(ctx, first.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	app, e := s.App(ctx, first.ApplicationID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Dispatch(ctx, op.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Applied(ctx, op, app, "stable-uid"); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	pending, e := s.Pending(ctx, 30)
	if e != nil || len(pending) != 1 || pending[0].ID != op.ID || pending[0].CommandID != op.CommandID || pending[0].DeadlineAt != op.DeadlineAt || pending[0].State != "RUNNING" {
		t.Fatalf("lost recovered operation %+v %v", pending, e)
	}
	d, e := s.Detail(ctx, domain.TenantID, app.ID, 100)
	if e != nil || len(d.Releases) != 1 || d.Application.KubernetesUID != "stable-uid" || d.Application.AppliedRevision != 1 {
		t.Fatalf("lost DB state %+v %v", d, e)
	}
	clusters, e := s.Clusters(ctx)
	if e != nil || clusters[0].State != "DISCONNECTED" {
		t.Fatalf("stale connected state %v %v", clusters, e)
	}
}

func TestVersionOneMigrationPreservesHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "version-one.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, migration); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `PRAGMA user_version=1;
INSERT INTO tenants VALUES('demo','Demo','demo',1);
INSERT INTO clusters VALUES('local-kind','Kind','CONNECTED',1,1);
INSERT INTO applications(id,tenant_id,cluster_id,name,status) VALUES('old-app','demo','local-kind','old','READY');
INSERT INTO operations(id,app_id,command_id,type,state,created_at,deadline_at) VALUES('old-op','old-app','old-command','CREATE','SUCCEEDED',1,2);
INSERT INTO operation_events VALUES('old-event','old-op','SUCCEEDED','old success',1);`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d, err := s.Detail(ctx, domain.TenantID, "old-app", 100)
	if err != nil {
		t.Fatal(err)
	}
	if d.Application.ObservationErrorCode != "" || len(d.Events) != 1 || d.Events[0].ID != "old-event" || d.Operations[0].CommandID != "old-command" {
		t.Fatalf("migration changed old history: %+v", d)
	}
	if _, err = s.db.ExecContext(ctx, "INSERT INTO operation_events VALUES('duplicate','old-op','SUCCEEDED','duplicate',2)"); err == nil {
		t.Fatal("migration removed lifecycle event deduplication")
	}
}

func TestObservationCannotRebindUIDOrOverwriteNewIntent(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	created := accept(t, s, intent("observation-guards"))
	finish(t, s, created, "SUCCEEDED")
	old, err := s.App(ctx, created.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Observe(ctx, old, domain.Observation{UID: "foreign-uid", Applied: true, Ready: true}); err == nil {
		t.Fatal("observation rebound a foreign UID")
	}
	if err = s.Observe(ctx, old, domain.Observation{Applied: true}); err == nil {
		t.Fatal("applied observation accepted without UID")
	}
	update := intent("unused")
	update.Kind = "RELEASE"
	update.AppID = old.ID
	update.Expected = 1
	update.Key = "new"
	update.Hash = "new"
	accept(t, s, update)
	if err = s.Observe(ctx, old, domain.Observation{UID: "k8s-uid", Applied: true, Ready: true}); err != nil {
		t.Fatal(err)
	}
	d, err := s.Detail(ctx, domain.TenantID, old.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if d.Application.DesiredRevision != 2 || d.Application.AppliedRevision != 1 || d.Application.Status != "DEPLOYING" || d.Application.LastSuccessfulRevision != 1 {
		t.Fatalf("stale observation overwrote a newer intent: %+v", d.Application)
	}
}

func TestDeleteRetainsHistoryAndLeavesObservationQueue(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	first := accept(t, s, intent("deleted"))
	finish(t, s, first, "SUCCEEDED")
	deletion := domain.Change{TenantID: domain.TenantID, AppID: first.ApplicationID, Kind: "DELETE", Expected: 1, Key: "delete", Hash: "delete"}
	accepted := accept(t, s, deletion)
	op, e := s.Operation(ctx, accepted.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.App(ctx, first.ApplicationID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(ctx, op, a, "SUCCEEDED", "", "GC verified by transport"); e != nil {
		t.Fatal(e)
	}
	replay := accept(t, s, deletion)
	if replay.OperationID != accepted.OperationID || replay.Revision != 1 {
		t.Fatalf("delete replay changed identity or revision %+v", replay)
	}
	d, e := s.Detail(ctx, domain.TenantID, a.ID, 100)
	if e != nil || d.Application.Status != "DELETED" || len(d.Releases) != 1 || len(d.Operations) != 2 || d.Application.LastSuccessfulRevision != 1 {
		t.Fatalf("deletion lost history %+v %v", d, e)
	}
	apps, e := s.ObservationApps(ctx)
	if e != nil || len(apps) != 0 {
		t.Fatalf("deleted history remains in observation queue %+v %v", apps, e)
	}
}
