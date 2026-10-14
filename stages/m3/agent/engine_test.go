package agent

import (
	"context"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m3/agentprotocol"
	pb "github.com/win30221/hugofleet/stages/m3/api/agent/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
	"time"
)

type work struct {
	spec            platform.ApplicationSpec
	uid             string
	writes, deletes int
}

func (w *work) Apply(_ context.Context, a domain.Application, s platform.ApplicationSpec) (string, error) {
	if w.uid != "" && s.Revision < w.spec.Revision {
		return "", domain.Problem("STALE_REVISION", "older CR")
	}
	w.spec = s
	w.uid = "uid-1"
	w.writes++
	return w.uid, nil
}
func (w *work) Observe(context.Context, domain.Application, platform.ApplicationSpec) (domain.Observation, error) {
	return domain.Observation{UID: w.uid, Applied: w.uid != ""}, nil
}
func (w *work) DeletionStatus(_ context.Context, a domain.Application) (domain.Observation, error) {
	if w.uid != "" && w.uid != a.KubernetesUID {
		return domain.Observation{}, domain.Problem("OWNERSHIP_CONFLICT", "UID differs")
	}
	return domain.Observation{UID: w.uid, Deleted: w.uid == ""}, nil
}
func (w *work) Delete(_ context.Context, a domain.Application) (domain.Observation, error) {
	w.uid = ""
	w.deletes++
	return domain.Observation{Deleted: true}, nil
}
func testCommand(rev int64) *pb.HubEnvelope {
	spec, _ := domain.DefaultPolicy().Spec(domain.DeployRequest{Image: "pricing-api:v1"})
	spec.Revision = rev
	a := domain.Application{ID: "app-1", TenantID: "demo", Name: "pricing", DesiredRevision: rev}
	c := &pb.HubEnvelope{CommandId: "command-1", OperationId: "op-1", ExpiresAt: time.Now().Add(time.Minute).UnixMilli(), Payload: &pb.HubEnvelope_Apply{Apply: &pb.ApplyApplication{Application: agentprotocol.Identity(a), Spec: agentprotocol.Spec(spec)}}}
	c.PayloadHash = agentprotocol.PayloadHash(c)
	return c
}

func TestTenantAllowlistAndLedgerIdentity(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: LedgerName, Namespace: "hf-system"}}).Build()
	w := &work{}
	engine := &Engine{Cluster: "hugofleet-a", Tenants: map[string]bool{"team-a": true, "team-b": true}, Ledger: &KubeLedger{Client: c, Namespace: "hf-system"}, Workloads: w}
	command := testCommand(1)
	command.GetApply().Application.TenantId = "team-a"
	command.GetApply().Application.Namespace = "hf-team-a"
	command.PayloadHash = agentprotocol.PayloadHash(command)
	if r := engine.Execute(ctx, command); r.ErrorCode != "" || w.writes != 1 {
		t.Fatalf("approved tenant rejected %+v", r)
	}
	foreign := testCommand(2)
	foreign.GetApply().Application.TenantId = "team-b"
	foreign.GetApply().Application.Namespace = "hf-team-b"
	foreign.PayloadHash = agentprotocol.PayloadHash(foreign)
	if r := engine.Execute(ctx, foreign); r.ErrorCode != "OWNERSHIP_CONFLICT" || w.writes != 1 {
		t.Fatalf("app_id crossed tenant ledger %+v", r)
	}
	foreign.GetApply().Application.ApplicationId = "other-app"
	foreign.GetApply().Application.TenantId = "unapproved"
	foreign.GetApply().Application.Namespace = "hf-unapproved"
	foreign.PayloadHash = agentprotocol.PayloadHash(foreign)
	if r := engine.Execute(ctx, foreign); r.ErrorCode != "PERMISSION_DENIED" || w.writes != 1 {
		t.Fatalf("unapproved namespace accepted %+v", r)
	}
	foreign.GetApply().Application.TenantId = "team-a"
	foreign.GetApply().Application.Namespace = "hf-team-b"
	foreign.PayloadHash = agentprotocol.PayloadHash(foreign)
	if r := engine.Execute(ctx, foreign); r.ErrorCode != "PERMISSION_DENIED" || w.writes != 1 {
		t.Fatalf("namespace spoof accepted %+v", r)
	}
}
func TestDurableHighWaterACKLossAndTombstone(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: LedgerName, Namespace: domain.Namespace}}).Build()
	w := &work{}
	restart := func() *Engine { return &Engine{Cluster: "hugofleet-a", Ledger: &KubeLedger{Client: c}, Workloads: w} }
	e := restart()
	command := testCommand(1)
	if r := e.Execute(ctx, command); r.ErrorCode != "" || r.Status.Uid != "uid-1" {
		t.Fatalf("apply %+v", r)
	}
	// Simulate ACK loss: same durable command after a fresh Agent/ledger instance.
	e = restart()
	if r := e.Execute(ctx, command); r.ErrorCode != "" {
		t.Fatalf("replay %+v", r)
	}
	changed := testCommand(1)
	changed.GetApply().Spec.Image = "pricing-api:v2-good"
	changed.PayloadHash = agentprotocol.PayloadHash(changed)
	if r := e.Execute(ctx, changed); r.ErrorCode != "CONFLICT" {
		t.Fatalf("conflicting command accepted %+v", r)
	}
	changed.CommandId = "other"
	changed.PayloadHash = agentprotocol.PayloadHash(changed)
	if r := e.Execute(ctx, changed); r.ErrorCode != "CONFLICT" {
		t.Fatalf("conflicting revision accepted %+v", r)
	}
	newer := testCommand(2)
	newer.CommandId = "command-2"
	if r := e.Execute(ctx, newer); r.ErrorCode != "" {
		t.Fatal(r)
	}
	e = restart()
	if r := e.Execute(ctx, command); r.ErrorCode != "STALE_REVISION" {
		t.Fatalf("old revision accepted %+v", r)
	}
	expired := testCommand(3)
	expired.ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	if r := e.Execute(ctx, expired); r.ErrorCode != "EXPIRED" {
		t.Fatal(r)
	}
	id := newer.GetApply().Application
	id.Uid = "uid-1"
	del := &pb.HubEnvelope{CommandId: "delete-2", OperationId: "delete-op", ExpiresAt: time.Now().Add(time.Minute).UnixMilli(), Payload: &pb.HubEnvelope_Delete{Delete: &pb.DeleteApplication{Application: id}}}
	del.PayloadHash = agentprotocol.PayloadHash(del)
	del.ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	if r := e.Execute(ctx, del); r.ErrorCode != "EXPIRED" || w.deletes != 0 {
		t.Fatal(r)
	}
	del.ExpiresAt = time.Now().Add(time.Minute).UnixMilli()
	if r := e.Execute(ctx, del); r.ErrorCode != "" {
		t.Fatal(r)
	}
	e = restart()
	if r := e.Execute(ctx, del); r.ErrorCode != "" || w.deletes != 1 {
		t.Fatalf("duplicate delete %+v; calls=%d", r, w.deletes)
	}
	resurrection := testCommand(3)
	if r := e.Execute(ctx, resurrection); r.ErrorCode != "DELETED" {
		t.Fatalf("resurrected %+v", r)
	}
	entries, err := e.Ledger.Load(ctx)
	if err != nil || !entries["app-1"].Deleted || entries["app-1"].Revision != 2 {
		t.Fatalf("tombstone missing %+v %v", entries, err)
	}
}
