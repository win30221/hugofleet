package repository

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTraceAndObservedSnapshotSurviveRestartAndReplay(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "hub.db")
	s, err := Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	if err = s.Connectivity(ctx, true); err != nil {
		t.Fatal(err)
	}
	id, _ := trace.TraceIDFromHex("11111111111111111111111111111111")
	span, _ := trace.SpanIDFromHex("2222222222222222")
	state, err := trace.ParseTraceState("hf=demo")
	if err != nil {
		t.Fatal(err)
	}
	parent := trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{TraceID: id, SpanID: span, TraceFlags: trace.FlagsSampled, TraceState: state}))
	c := intent("trace-app")
	a, err := s.Accept(parent, c, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	o, err := s.Operation(ctx, a.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	wantParent, wantState := observability.Inject(parent)
	if o.Traceparent != wantParent || o.Tracestate != wantState {
		t.Fatalf("accepted context not persisted: %+v", o)
	}
	app, err := s.App(ctx, a.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	ob := domain.Observation{ObservedRevision: 1, Generation: 7, ObservedGeneration: 7, UID: "real-fixture-uid", Applied: true, Ready: true, Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "RolloutComplete", Message: "exact revision ready", ObservedGeneration: 7, LastTransitionTime: metav1.NewTime(time.UnixMilli(1234567890000))}}}
	if err = s.Observe(ctx, app, ob); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute).UnixMilli()
	if _, err = s.db.ExecContext(ctx, "UPDATE applications SET last_observed_at=? WHERE id=?", old, app.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Unknown(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.App(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "UNKNOWN" || current.LastObservedAt != old || !reflect.DeepEqual(current.Observed, ob) {
		t.Fatalf("restart fabricated or dropped observation: %+v", current)
	}
	if err = s.Connectivity(ctx, true); err != nil {
		t.Fatal(err)
	}
	replay, err := s.Accept(ctx, c, time.Minute)
	if err != nil || replay != a {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	persisted, err := s.Operation(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Traceparent != wantParent || persisted.Tracestate != wantState {
		t.Fatal("replay replaced the original trace")
	}
}

func TestTerminalHookOnlyAfterCommitAndExactlyOnce(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	a := accept(t, s, intent("metrics-app"))
	o, err := s.Operation(ctx, a.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	app, err := s.App(ctx, a.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	s.Terminal = func(v domain.Operation, x domain.Application) {
		count++
		if v.State != "SUCCEEDED" || v.FinishedAt < v.CreatedAt || x.ID != app.ID {
			t.Fatal("hook has no committed outcome")
		}
	}
	if _, err = s.db.ExecContext(ctx, "CREATE TRIGGER reject_terminal BEFORE UPDATE OF state ON operations WHEN NEW.state='SUCCEEDED' BEGIN SELECT RAISE(ABORT,'fixture transaction failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, o, app, "SUCCEEDED", "", "done"); err == nil || count != 0 {
		t.Fatal("failed write emitted a terminal metric")
	}
	if _, err = s.db.ExecContext(ctx, "DROP TRIGGER reject_terminal"); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, o, app, "SUCCEEDED", "", "done"); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, o, app, "FAILED", "late", "late failure"); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("terminal hook count=%d", count)
	}
	persisted, err := s.Operation(ctx, o.ID)
	if err != nil || persisted.State != "SUCCEEDED" {
		t.Fatal("late observation rewrote committed history")
	}
}
