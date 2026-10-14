package service

import (
	"context"
	"errors"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"path/filepath"
	"testing"
	"time"
)

// These runner tests use real SQLite and a controlled transport. Real CR
// identity, rollout and crash recovery are independently verified by kind E2E.
type observationTransport struct {
	ob                    domain.Observation
	err                   error
	observations, applies int
}

func (*observationTransport) Ping(context.Context) error { return nil }
func (t *observationTransport) Apply(context.Context, domain.Application, platform.ApplicationSpec) (string, error) {
	t.applies++
	return "cr-uid", nil
}
func (t *observationTransport) Observe(context.Context, domain.Application, platform.ApplicationSpec) (domain.Observation, error) {
	t.observations++
	return t.ob, t.err
}
func (*observationTransport) Delete(context.Context, domain.Application) (domain.Observation, error) {
	return domain.Observation{}, nil
}

func TestRestartAfterDeadlineStillObservesUnacknowledgedCR(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hub.db")
	store, err := repository.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Connectivity(ctx, true); err != nil {
		t.Fatal(err)
	}
	svc := Service{store, domain.DefaultPolicy(), time.Millisecond}
	zero := int64(0)
	accepted, err := svc.Deploy(ctx, domain.TenantID, "", "lost-ack", domain.DeployRequest{Name: "lost-ack", ClusterID: domain.ClusterID, Image: "pricing-api:v1", ExpectedDesiredRevision: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Dispatch(ctx, accepted.OperationID); err != nil {
		t.Fatal(err)
	}
	before, err := store.Operation(ctx, accepted.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	store, err = repository.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	transport := &observationTransport{ob: domain.Observation{UID: "cr-uid", Applied: true, Ready: true}}
	runner := Runner{Store: store, Transport: transport, Interval: time.Second}
	for range 3 {
		if err = runner.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	d, err := store.Detail(ctx, domain.TenantID, accepted.ApplicationID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if transport.observations != 3 || transport.applies != 0 {
		t.Fatalf("read-only recovery: observes=%d applies=%d", transport.observations, transport.applies)
	}
	if d.Application.Status != "READY" || d.Application.LastSuccessfulRevision != 0 || d.Operations[0].State != "TIMED_OUT" || d.Releases[0].State != "FAILED" {
		t.Fatalf("recovery lost current state or rewrote history: %+v", d)
	}
	if d.Application.AppliedRevision != 1 || d.Application.KubernetesUID != "cr-uid" {
		t.Fatalf("lost CR acknowledgement not recovered: %+v", d.Application)
	}
	if d.Operations[0].CommandID != before.CommandID || d.Operations[0].DeadlineAt != before.DeadlineAt {
		t.Fatal("recovery replaced command or deadline")
	}
	late := 0
	for _, event := range d.Events {
		if event.Kind == "LATE_READY" {
			late++
		}
	}
	if late != 1 {
		t.Fatalf("late readiness should be recorded once: %+v", d.Events)
	}
}

func TestValidationAndRequestHash(t *testing.T) {
	ctx := context.Background()
	store, e := repository.Open(ctx, filepath.Join(t.TempDir(), "hub.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if e = store.Connectivity(ctx, true); e != nil {
		t.Fatal(e)
	}
	s := Service{store, domain.DefaultPolicy(), time.Minute}
	zero := int64(0)
	r := domain.DeployRequest{Name: "pricing-api", ClusterID: domain.ClusterID, Image: "pricing-api:v1", ExpectedDesiredRevision: &zero}
	check := func(e error, code string) {
		t.Helper()
		var p *domain.Error
		if !errors.As(e, &p) || p.Code != code {
			t.Fatalf("%v want %s", e, code)
		}
	}
	_, e = s.Deploy(ctx, domain.TenantID, "", "", r)
	check(e, "INVALID_ARGUMENT")
	bad := r
	bad.ExpectedDesiredRevision = nil
	_, e = s.Deploy(ctx, domain.TenantID, "", "key", bad)
	check(e, "INVALID_ARGUMENT")
	bad = r
	bad.Name = "INVALID"
	_, e = s.Deploy(ctx, domain.TenantID, "", "key", bad)
	check(e, "INVALID_ARGUMENT")
	bad = r
	bad.Resources.Limits.CPU = "2"
	_, e = s.Deploy(ctx, domain.TenantID, "", "key", bad)
	check(e, "QUOTA_EXCEEDED")
	first, e := s.Deploy(ctx, domain.TenantID, "", "key", r)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Deploy(ctx, domain.TenantID, "", "key", r)
	if e != nil || first.OperationID != again.OperationID {
		t.Fatalf("dedup failed %+v %v", again, e)
	}
	changed := s
	changed.Policy.MaxCPU = "100m"
	replay, e := changed.Deploy(ctx, domain.TenantID, "", "key", r)
	if e != nil || replay.OperationID != first.OperationID {
		t.Fatalf("changed policy broke accepted request replay %+v %v", replay, e)
	}
	r.Image = "pricing-api:v2-good"
	_, e = s.Deploy(ctx, domain.TenantID, "", "key", r)
	check(e, "CONFLICT")
	d, e := store.Detail(ctx, domain.TenantID, first.ApplicationID, 100)
	if e != nil || len(d.Releases) != 1 || len(d.Operations) != 1 {
		t.Fatalf("invalid request created rows: %+v %v", d, e)
	}
}
