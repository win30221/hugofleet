//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	security "github.com/win30221/hugofleet/stages/m4/test/e2e"
	"github.com/win30221/hugofleet/stages/m4/testdriver"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"github.com/win30221/hugofleet/stages/m5/traceview"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type suite struct {
	t                *testing.T
	ctx              context.Context
	state, a, b, hub string
	sessions         map[string]*testdriver.ManagementSession
	workers          map[string]client.Client
}

func TestM5RealSecurity(t *testing.T) { security.RunRealTenantSecurity(t, "m5") }

// Runs after real security acceptance on the same owned state. The formal entry
// runs both tests; development can resume this scope without replaying CREATE.
func TestM5RealDashboardTracesAndHelm(t *testing.T) {
	s := newSuite(t)
	s.dashboard()
	s.traces()
	s.helmLifecycle()
}

func newSuite(t *testing.T) *suite {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	t.Cleanup(cancel)
	s := &suite{t: t, ctx: ctx, state: os.Getenv("HF_M5_STATE"), a: os.Getenv("HF_M5_A"), b: os.Getenv("HF_M5_B"), hub: os.Getenv("HF_M5_HUB"), sessions: map[string]*testdriver.ManagementSession{}, workers: map[string]client.Client{}}
	if s.state == "" || s.a == "" || s.b == "" || (s.hub != "hugofleet-hub-m5-e2e" && s.hub != "hugofleet-hub-m5-development") {
		t.Fatal("exact real owned M5 environment required")
	}
	settings, err := testdriver.ReadSettings(filepath.Join(s.state, "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range settings.Accounts {
		v, e := testdriver.LoginHub(ctx, settings.OIDC, account)
		if e != nil {
			t.Fatalf("fresh real session %s: %v", account.Name, e)
		}
		s.sessions[account.Name] = v
	}
	for _, id := range []string{s.a, s.b} {
		path := filepath.Join(s.state, "clusters", id, "kubeconfig")
		raw, e := clientcmd.LoadFromFile(path)
		if e != nil || raw.CurrentContext != "kind-"+id {
			t.Fatal("wrong owned Worker context")
		}
		cfg, e := clientcmd.NewNonInteractiveClientConfig(*raw, "kind-"+id, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
		if e != nil {
			t.Fatal(e)
		}
		cfg.Timeout = 5 * time.Second
		scheme := runtime.NewScheme()
		for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme, platform.AddToScheme} {
			if e = add(scheme); e != nil {
				t.Fatal(e)
			}
		}
		c, e := client.New(cfg, client.Options{Scheme: scheme})
		if e != nil {
			t.Fatal(e)
		}
		s.workers[id] = c
	}
	return s
}
func (s *suite) api(who, method, path, key string, body any, want int, out any) []byte {
	s.t.Helper()
	status, raw, err := s.sessions[who].Request(s.ctx, method, path, key, body)
	if err != nil || status != want {
		s.t.Fatalf("actual %s %s expected=%d status=%d error=%v", method, path, want, status, err)
	}
	if out != nil {
		if err = json.Unmarshal(raw, out); err != nil {
			s.t.Fatal(err)
		}
	}
	return raw
}
func (s *suite) eventually(label string, timeout time.Duration, f func() bool) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		select {
		case <-s.ctx.Done():
			s.t.Fatal(s.ctx.Err())
		case <-time.After(time.Second):
		}
	}
	s.t.Fatal("actual verification timed out: " + label)
}
func (s *suite) wait(a domain.Accepted) domain.Detail {
	s.t.Helper()
	var d domain.Detail
	s.eventually("terminal deployment", 3*time.Minute, func() bool {
		var op domain.Operation
		s.api("owner-a", "GET", "/api/v1/operations/"+a.OperationID, "", nil, 200, &op)
		if op.State == "FAILED" || op.State == "TIMED_OUT" {
			s.t.Fatalf("actual deployment failed: %s %s", op.ErrorCode, op.ErrorMessage)
		}
		s.api("owner-a", "GET", "/api/v1/tenants/team-a/apps/"+a.ApplicationID, "", nil, 200, &d)
		return op.State == "SUCCEEDED" && d.Application.Status == "READY"
	})
	return d
}
func (s *suite) dashboard() {
	for _, path := range []string{"/", "/clusters", "/applications", "/applications?tenant=team-a&cluster=" + s.a} {
		raw := s.api("owner-a", "GET", path, "", nil, 200, nil)
		if len(raw) < 1000 || !strings.Contains(string(raw), "HugoFleet") {
			s.t.Fatal("actual Dashboard unavailable")
		}
	}
	for _, path := range []string{"/metrics", "/traces", "/api/v1/traces", "/applications?tenant=team-b"} {
		s.api("viewer-a", "GET", path, "", nil, 403, nil)
	}
	var apps domain.Page[domain.Application]
	s.api("owner-a", "GET", "/api/v1/tenants/team-a/apps", "", nil, 200, &apps)
	for _, a := range apps.Items {
		if a.Name != "pricing-api" || a.ClusterID != s.a {
			continue
		}
		s.eventually("Dashboard snapshot equals actual CR", time.Minute, func() bool {
			var d domain.Detail
			s.api("owner-a", "GET", "/api/v1/tenants/team-a/apps/"+a.ID, "", nil, 200, &d)
			cr := &platform.Application{}
			if err := s.workers[s.a].Get(s.ctx, client.ObjectKey{Namespace: "hf-team-a", Name: a.Name}, cr); err != nil {
				s.t.Fatal(err)
			}
			x, y := jsonBytes(s.t, d.Application.Observed.Conditions), jsonBytes(s.t, cr.Status.Conditions)
			return d.Application.Observed.UID == string(cr.UID) && d.Application.Observed.ObservedRevision == cr.Spec.Revision && d.Application.Observed.Generation == cr.Generation && d.Application.Observed.ObservedGeneration == cr.Status.ObservedGeneration && string(x) == string(y)
		})
		raw := s.api("owner-a", "GET", "/tenants/team-a/apps/"+a.ID, "", nil, 200, nil)
		if !strings.Contains(string(raw), "Worker 觀測證據") || !strings.Contains(string(raw), "RolloutComplete") {
			s.t.Fatal("actual Conditions absent from HTML")
		}
		s.t.Log("PASS Dashboard actual CR UID/revision/generation/Conditions plus scoped pages and Admin-only telemetry")
		return
	}
	s.t.Fatal("actual baseline application missing")
}
func jsonBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func metric(raw []byte, key string) (float64, error) {
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, key+" ") {
			return strconv.ParseFloat(strings.TrimPrefix(line, key+" "), 64)
		}
	}
	return 0, fmt.Errorf("metric unavailable")
}
func (s *suite) traces() {
	const counter = `hugofleet_operations_total{state="SUCCEEDED",type="CREATE"}`
	before, err := metric(s.api("admin", "GET", "/metrics", "", nil, 200, nil), counter)
	if err != nil {
		s.t.Fatal(err)
	}
	accepted := []domain.Accepted{}
	ops := []domain.Operation{}
	zero := int64(0)
	for i := 0; i < 2; i++ {
		var a domain.Accepted
		s.api("owner-a", "POST", "/api/v1/tenants/team-a/apps", uuid.NewString(), domain.DeployRequest{Name: "trace-" + uuid.NewString()[:8], ClusterID: s.a, ExpectedDesiredRevision: &zero, Image: "pricing-api:v1"}, 202, &a)
		s.wait(a)
		var op domain.Operation
		s.api("owner-a", "GET", "/api/v1/operations/"+a.OperationID, "", nil, 200, &op)
		accepted = append(accepted, a)
		ops = append(ops, op)
	}
	if ops[0].Traceparent == "" || ops[1].Traceparent == "" || strings.Split(ops[0].Traceparent, "-")[1] == strings.Split(ops[1].Traceparent, "-")[1] {
		s.t.Fatal("different actual operations share a trace")
	}
	after, err := metric(s.api("admin", "GET", "/metrics", "", nil, 200, nil), counter)
	if err != nil || after-before != 2 {
		s.t.Fatalf("actual committed counter delta=%v err=%v", after-before, err)
	}
	var hubRecords []observability.Record
	s.eventually("Hub command spans exported", time.Minute, func() bool {
		hubRecords = nil
		found := map[string]bool{}
		for _, op := range ops {
			var records []observability.Record
			traceID := strings.Split(op.Traceparent, "-")[1]
			s.api("admin", "GET", "/api/v1/traces?trace="+traceID, "", nil, 200, &records)
			hubRecords = append(hubRecords, records...)
			for _, r := range records {
				if r.Name == "hub.command" && r.Attributes["operation_id"] == op.ID {
					found[op.ID] = true
				}
			}
		}
		return len(found) == 2
	})
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	kubeconfig := filepath.Join(s.state, "clusters", s.a, "kubeconfig")
	root := os.Getenv("HF_ROOT")
	log, err := exec.CommandContext(ctx, filepath.Join(root, ".tools/bin/kubectl"), "--kubeconfig", kubeconfig, "--context", "kind-"+s.a, "-n", "hf-system", "logs", "deployment/hugofleet-agent").Output()
	if err != nil {
		s.t.Fatal(err)
	}
	agentRecords, err := traceview.Read(strings.NewReader(string(log)), 20000)
	if err != nil {
		s.t.Fatal(err)
	}
	channels := map[string]bool{}
	for _, op := range ops {
		found := false
		for _, a := range agentRecords {
			if a.Name != "agent.command" || a.Attributes["operation_id"] != op.ID {
				continue
			}
			for _, h := range hubRecords {
				if h.Name == "hub.command" && h.TraceID == a.TraceID && h.SpanID == a.ParentID && h.Attributes["operation_id"] == op.ID && h.Attributes["channel_id"] == a.Attributes["channel_id"] && a.Attributes["channel_id"] != "" {
					found = true
					channels[a.Attributes["channel_id"]] = true
				}
			}
		}
		if !found {
			s.t.Fatal("actual Agent span has no matching envelope parent/channel")
		}
	}
	if len(channels) != 1 {
		s.t.Fatal("two operation proofs did not use the same actual bidi stream")
	}
	for _, a := range accepted {
		expected := int64(1)
		var deleted domain.Accepted
		s.api("owner-a", "DELETE", "/api/v1/tenants/team-a/apps/"+a.ApplicationID, uuid.NewString(), domain.DeleteRequest{ExpectedDesiredRevision: &expected}, 202, &deleted)
		s.eventually("trace fixture deletion", time.Minute, func() bool {
			var op domain.Operation
			s.api("owner-a", "GET", "/api/v1/operations/"+deleted.OperationID, "", nil, 200, &op)
			return op.State == "SUCCEEDED"
		})
	}
	s.t.Log("PASS two real OIDC/SQLite/mTLS operations: distinct trace IDs on one actual channel, matching Hub→Agent span parents, committed metrics delta=2 and retained deletion tombstones")
}

func (s *suite) helmLifecycle() {
	w := s.workers[s.a]
	ledger := &corev1.ConfigMap{}
	key := client.ObjectKey{Namespace: "hf-system", Name: "hugofleet-agent-ledger"}
	if err := w.Get(s.ctx, key, ledger); err != nil {
		s.t.Fatal(err)
	}
	uid, data := ledger.UID, jsonBytes(s.t, ledger.Data)
	receipts := map[string]corev1.ConfigMap{}
	for _, ns := range []string{"hf-team-a", "hf-team-b"} {
		v := corev1.ConfigMap{}
		if err := w.Get(s.ctx, client.ObjectKey{Namespace: ns, Name: "hugofleet-tenant-bootstrap"}, &v); err != nil {
			s.t.Fatal(err)
		}
		receipts[ns] = v
	}
	root := os.Getenv("HF_ROOT")
	kubeconfig := filepath.Join(s.state, "clusters", s.a, "kubeconfig")
	helm := func(args ...string) {
		s.t.Helper()
		ctx, cancel := context.WithTimeout(s.ctx, 6*time.Minute)
		defer cancel()
		args = append(args, "--namespace", "hf-system", "--kubeconfig", kubeconfig, "--kube-context", "kind-"+s.a)
		if out, err := exec.CommandContext(ctx, filepath.Join(root, ".tools/bin/helm"), args...).CombinedOutput(); err != nil {
			s.t.Fatalf("actual owned Helm action failed: %v %s", err, out)
		}
	}
	values := filepath.Join(s.state, s.a+"-values.json")
	chart := filepath.Join(root, "stages/m5/chart")
	helm("upgrade", "hugofleet-worker", chart, "--values", values, "--set", "rollout=2", "--wait=watcher", "--timeout=5m")
	helm("uninstall", "hugofleet-worker", "--wait=watcher", "--timeout=5m")
	if err := w.Get(s.ctx, key, ledger); err != nil || ledger.UID != uid || string(jsonBytes(s.t, ledger.Data)) != string(data) {
		s.t.Fatal("Helm uninstall deleted or reset the actual command ledger")
	}
	for ns, v := range receipts {
		current := corev1.ConfigMap{}
		if err := w.Get(s.ctx, client.ObjectKey{Namespace: ns, Name: v.Name}, &current); err != nil || current.UID != v.UID || !reflect.DeepEqual(current.Data, v.Data) {
			s.t.Fatal("Helm uninstall deleted or reset trusted receipt")
		}
		if err := w.Get(s.ctx, client.ObjectKey{Namespace: ns, Name: "hugofleet-quota"}, &corev1.ResourceQuota{}); err != nil {
			s.t.Fatal("Helm uninstall removed Admission isolation")
		}
	}
	helm("install", "hugofleet-worker", chart, "--values", values, "--wait=watcher", "--timeout=5m")
	s.eventually("actual Agent reconnect after Helm reinstall", time.Minute, func() bool {
		var c []domain.Cluster
		s.api("admin", "GET", "/api/v1/clusters", "", nil, 200, &c)
		for _, v := range c {
			if v.ID == s.a {
				return v.State == "CONNECTED" && v.AgentVersion == "m5"
			}
		}
		return false
	})
	if err := w.Get(s.ctx, key, ledger); err != nil || ledger.UID != uid || string(jsonBytes(s.t, ledger.Data)) != string(data) {
		s.t.Fatal("Helm reinstall reset high-water/tombstone data")
	}
	s.t.Log("PASS actual Helm upgrade/uninstall/reinstall with exact release, original ledger UID/full high-water/tombstones and receipt UID/data; Admission material retained and real Agent reconnected")
}
