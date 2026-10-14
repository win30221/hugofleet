//go:build e2e

package m3e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/kube"
	"github.com/win30221/hugofleet/stages/m3/agent"
	"github.com/win30221/hugofleet/stages/m3/agentprotocol"
	pb "github.com/win30221/hugofleet/stages/m3/api/agent/v1"
	"io"
	appsv1 "k8s.io/api/apps/v1"
	authv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"testing"
	"time"
)

type harness struct {
	t                        *testing.T
	ctx                      context.Context
	root, state, base, token string
	c                        map[string]client.Client
	admin                    map[string]*kubernetes.Clientset
}

func (h *harness) wait(label string, f func() error) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(h.ctx, 180*time.Second)
	defer cancel()
	var last error
	for {
		last = f()
		if last == nil {
			h.t.Log("PASS", label)
			return
		}
		select {
		case <-ctx.Done():
			h.t.Fatalf("%s: %v; last=%v", label, ctx.Err(), last)
		case <-time.After(500 * time.Millisecond):
		}
	}
}
func (h *harness) request(method, path, key string, payload any) (int, []byte, error) {
	var b []byte
	if payload != nil {
		b, _ = json.Marshal(payload)
	}
	r, e := http.NewRequestWithContext(h.ctx, method, h.base+path, bytes.NewReader(b))
	if e != nil {
		return 0, nil, e
	}
	r.Header.Set("Authorization", "Bearer "+h.token)
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	if payload != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	res, e := (&http.Client{Timeout: 10 * time.Second}).Do(r)
	if e != nil {
		return 0, nil, e
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	return res.StatusCode, raw, e
}
func (h *harness) accept(method, path string, v any) domain.Accepted {
	h.t.Helper()
	code, raw, e := h.request(method, path, uuid.NewString(), v)
	if e != nil || code != 202 {
		h.t.Fatalf("accept %d %s %v", code, raw, e)
	}
	var o domain.Accepted
	if e = json.Unmarshal(raw, &o); e != nil {
		h.t.Fatal(e)
	}
	return o
}
func (h *harness) detail(id string) (domain.Detail, error) {
	code, raw, e := h.request("GET", "/api/v1/tenants/demo/apps/"+id, "", nil)
	var d domain.Detail
	if e != nil {
		return d, e
	}
	if code != 200 {
		return d, fmt.Errorf("detail %d %s", code, raw)
	}
	e = json.Unmarshal(raw, &d)
	return d, e
}
func (h *harness) operation(id string) (domain.Operation, error) {
	code, raw, e := h.request("GET", "/api/v1/operations/"+id, "", nil)
	var o domain.Operation
	if e != nil {
		return o, e
	}
	if code != 200 {
		return o, fmt.Errorf("operation %d %s", code, raw)
	}
	e = json.Unmarshal(raw, &o)
	return o, e
}
func (h *harness) terminal(o domain.Accepted, state string) domain.Operation {
	h.t.Helper()
	var out domain.Operation
	h.wait("operation "+state, func() error {
		v, e := h.operation(o.OperationID)
		if e != nil {
			return e
		}
		out = v
		if v.State == state {
			return nil
		}
		if !v.Active() {
			h.t.Fatalf("unexpected terminal %+v", v)
		}
		return fmt.Errorf("state=%s", v.State)
	})
	return out
}
func (h *harness) clusters(want map[string]string) {
	h.t.Helper()
	h.wait("cluster registry "+fmt.Sprint(want), func() error {
		code, raw, e := h.request("GET", "/api/v1/clusters", "", nil)
		if e != nil {
			return e
		}
		if code != 200 {
			return fmt.Errorf("registry %d %s", code, raw)
		}
		var cs []domain.Cluster
		if e = json.Unmarshal(raw, &cs); e != nil {
			return e
		}
		if len(cs) != 2 {
			return fmt.Errorf("clusters=%d", len(cs))
		}
		for _, c := range cs {
			if c.State != want[c.ID] || c.LastSeenAt == 0 || c.AgentVersion != "m3" {
				return fmt.Errorf("cluster %+v", c)
			}
		}
		return nil
	})
}
func (h *harness) run(args ...string) {
	h.t.Helper()
	cmd := exec.CommandContext(h.ctx, args[0], args[1:]...)
	cmd.Dir = h.root
	raw, e := cmd.CombinedOutput()
	if e != nil {
		h.t.Fatalf("command %s failed: %s %v", args[0], raw, e)
	}
}
func (h *harness) scaleAgent(id string, n int32) {
	h.t.Helper()
	d, e := h.admin[id].AppsV1().Deployments("hf-demo").Get(h.ctx, "hugofleet-agent", metav1.GetOptions{})
	if e != nil {
		h.t.Fatal(e)
	}
	d.Spec.Replicas = &n
	if _, e = h.admin[id].AppsV1().Deployments("hf-demo").Update(h.ctx, d, metav1.UpdateOptions{}); e != nil {
		h.t.Fatal(e)
	}
	h.wait("Agent replicas "+fmt.Sprint(n), func() error {
		pods, e := h.admin[id].CoreV1().Pods("hf-demo").List(h.ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=hugofleet-agent"})
		if e != nil {
			return e
		}
		if n == 0 && len(pods.Items) == 0 {
			return nil
		}
		if n == 1 && len(pods.Items) == 1 && pods.Items[0].Status.Phase == corev1.PodRunning {
			return nil
		}
		return fmt.Errorf("pods=%d", len(pods.Items))
	})
}
func (h *harness) quote(id, name string, port int, wantVersion string) {
	h.t.Helper()
	log, e := os.Create(filepath.Join(os.Getenv("HF_ARTIFACTS"), fmt.Sprintf("forward-%s-%d.log", id, port)))
	if e != nil {
		h.t.Fatal(e)
	}
	defer log.Close()
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(h.root, ".tools/bin/kubectl"), "--kubeconfig", filepath.Join(h.root, ".local/kubeconfig-"+id), "--context", "kind-"+id, "-n", "hf-demo", "port-forward", "--address", "127.0.0.1", "service/"+name, fmt.Sprintf("%d:8080", port))
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		h.t.Fatal(e)
	}
	defer func() { cancel(); cmd.Wait() }()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	hc := &http.Client{Timeout: 3 * time.Second}
	h.wait("real "+id+" pricing-api /version "+wantVersion, func() error {
		res, e := hc.Get(base + "/version")
		if e != nil {
			return e
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 || !bytes.Contains(b, []byte(wantVersion)) {
			return fmt.Errorf("version %d %s", res.StatusCode, b)
		}
		return nil
	})
	res, e := hc.Post(base+"/quote", "application/json", strings.NewReader(`{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}`))
	if e != nil {
		h.t.Fatal(e)
	}
	defer res.Body.Close()
	var answer map[string]int64
	if e = json.NewDecoder(res.Body).Decode(&answer); e != nil || res.StatusCode != 200 || answer["final_amount_cents"] != 8000 {
		h.t.Fatalf("real quote %d %+v %v", res.StatusCode, answer, e)
	}
	h.t.Log("PASS real quote=8000", id)
}
func TestM3DualKindOutboundAgents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	root := os.Getenv("HF_ROOT")
	state := os.Getenv("HF_M3_STATE")
	a := os.Getenv("HF_M3_A")
	b := os.Getenv("HF_M3_B")
	token, e := os.ReadFile(filepath.Join(state, "certs/admin-token"))
	if e != nil {
		t.Fatal(e)
	}
	h := &harness{t: t, ctx: ctx, root: root, state: state, base: "http://127.0.0.1:" + os.Getenv("HF_M3_HTTP_PORT"), token: string(token), c: map[string]client.Client{}, admin: map[string]*kubernetes.Clientset{}}
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{platform.AddToScheme, corev1.AddToScheme, appsv1.AddToScheme} {
		if e = add(scheme); e != nil {
			t.Fatal(e)
		}
	}
	for _, id := range []string{a, b} {
		cfg, e := clientcmd.BuildConfigFromFlags("", filepath.Join(root, ".local/kubeconfig-"+id))
		if e != nil {
			t.Fatal(e)
		}
		cfg.Timeout = 5 * time.Second
		h.c[id], e = client.New(cfg, client.Options{Scheme: scheme})
		if e != nil {
			t.Fatal(e)
		}
		h.admin[id], e = kubernetes.NewForConfig(cfg)
		if e != nil {
			t.Fatal(e)
		}
	}
	h.clusters(map[string]string{a: "CONNECTED", b: "CONNECTED"})
	// docker inspect contains only file paths, never the mounted credential contents.
	raw, e := exec.CommandContext(ctx, "docker", "inspect", os.Getenv("HF_M3_HUB")).Output()
	if e != nil {
		t.Fatal(e)
	}
	var containers []struct {
		Config struct {
			Cmd []string
			Env []string
		}
		Mounts     []struct{ Destination string }
		HostConfig struct {
			PortBindings map[string][]struct{ HostIp string }
		}
	}
	if e = json.Unmarshal(raw, &containers); e != nil || len(containers) != 1 {
		t.Fatal(e)
	}
	allowed := map[string]bool{"/data": true, "/certs/ca.crt": true, "/certs/hub.crt": true, "/certs/hub.key": true, "/certs/admin-token": true}
	for _, m := range containers[0].Mounts {
		if !allowed[m.Destination] {
			t.Fatalf("unexpected Hub mount %s", m.Destination)
		}
	}
	if len(containers[0].Mounts) != 5 || strings.Contains(strings.Join(containers[0].Config.Cmd, " "), "kubeconfig") || strings.Contains(strings.Join(containers[0].Config.Env, " "), "KUBECONFIG") {
		t.Fatal("Hub has unexpected Worker credential configuration")
	}
	for _, ports := range containers[0].HostConfig.PortBindings {
		for _, p := range ports {
			if p.HostIp != "127.0.0.1" {
				t.Fatal("public management port", p)
			}
		}
	}
	t.Log("PASS Hub mounts only its credentials/database; no Worker kubeconfig")
	request, _ := http.NewRequestWithContext(ctx, "GET", h.base+"/api/v1/clusters", nil)
	response, e := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("unauthenticated management API accepted", response.StatusCode)
	}
	code, html, e := h.request("GET", "/", "", nil)
	if e != nil || code != 200 || !bytes.Contains(html, []byte(a)) || !bytes.Contains(html, []byte(b)) {
		t.Fatalf("UI cluster selection %d %s %v", code, html, e)
	}
	zero := int64(0)
	one := int64(1)
	first := h.accept("POST", "/api/v1/tenants/demo/apps", domain.DeployRequest{Name: "pricing-a", ClusterID: a, Image: "pricing-api:v1", ExpectedDesiredRevision: &zero})
	second := h.accept("POST", "/api/v1/tenants/demo/apps", domain.DeployRequest{Name: "pricing-b", ClusterID: b, Image: "pricing-api:v1", ExpectedDesiredRevision: &zero})
	h.terminal(first, "SUCCEEDED")
	h.terminal(second, "SUCCEEDED")
	h.quote(a, "pricing-a", 18280, "v1")
	h.quote(b, "pricing-b", 18281, "v1")
	path := "/api/v1/tenants/demo/apps/" + first.ApplicationID
	scaled := h.accept("POST", path+"/releases", domain.DeployRequest{Image: "pricing-api:v1", Replicas: 2, ExpectedDesiredRevision: &one})
	h.terminal(scaled, "SUCCEEDED")
	var before platform.Application
	if e = h.c[a].Get(ctx, client.ObjectKey{Namespace: "hf-demo", Name: "pricing-a"}, &before); e != nil {
		t.Fatal(e)
	}
	h.scaleAgent(a, 0)
	h.clusters(map[string]string{a: "DISCONNECTED", b: "CONNECTED"})
	h.wait("A UNKNOWN retains observation time", func() error {
		d, e := h.detail(first.ApplicationID)
		if e != nil {
			return e
		}
		if d.Application.Status != "UNKNOWN" || d.Application.LastObservedAt == 0 {
			return fmt.Errorf("app %+v", d.Application)
		}
		return nil
	})
	code, _, e = h.request("POST", "/api/v1/tenants/demo/apps", uuid.NewString(), domain.DeployRequest{Name: "offline", ClusterID: a, Image: "pricing-api:v1", ExpectedDesiredRevision: &zero})
	if e != nil || code != 503 {
		t.Fatalf("offline request %d %v", code, e)
	}
	dep, e := h.admin[a].AppsV1().Deployments("hf-demo").Get(ctx, "pricing-a", metav1.GetOptions{})
	if e != nil {
		t.Fatal(e)
	}
	dep.Spec.Replicas = new(int32(1))
	if _, e = h.admin[a].AppsV1().Deployments("hf-demo").Update(ctx, dep, metav1.UpdateOptions{}); e != nil {
		t.Fatal(e)
	}
	h.wait("Operator repairs replicas while Agent offline", func() error {
		d, e := h.admin[a].AppsV1().Deployments("hf-demo").Get(ctx, "pricing-a", metav1.GetOptions{})
		if e != nil {
			return e
		}
		if *d.Spec.Replicas != 2 || d.Status.ReadyReplicas != 2 {
			return fmt.Errorf("replicas %d ready %d", *d.Spec.Replicas, d.Status.ReadyReplicas)
		}
		return nil
	})
	updateB := h.accept("POST", "/api/v1/tenants/demo/apps/"+second.ApplicationID+"/releases", domain.DeployRequest{Image: "pricing-api:v2-good", ExpectedDesiredRevision: &one})
	h.terminal(updateB, "SUCCEEDED")
	h.quote(b, "pricing-b", 18281, "v2-good")
	h.probeReplay(a)
	h.scaleAgent(a, 1)
	h.clusters(map[string]string{a: "CONNECTED", b: "CONNECTED"})
	h.wait("reconnect restores A READY without duplicate apply", func() error {
		d, e := h.detail(first.ApplicationID)
		if e != nil {
			return e
		}
		var current platform.Application
		if e = h.c[a].Get(ctx, client.ObjectKey{Namespace: "hf-demo", Name: "pricing-a"}, &current); e != nil {
			return e
		}
		if current.UID != before.UID || current.Generation != before.Generation {
			t.Fatal("reconnect changed CR identity/generation")
		}
		if d.Application.Status != "READY" {
			return fmt.Errorf("status=%s", d.Application.Status)
		}
		return nil
	})
	two := int64(2)
	broken := h.accept("POST", path+"/releases", domain.DeployRequest{Image: "pricing-api:v2-broken", Replicas: 2, ExpectedDesiredRevision: &two})
	h.wait("broken revision applied before Hub restart", func() error {
		op, e := h.operation(broken.OperationID)
		if e != nil {
			return e
		}
		if op.State != "RUNNING" {
			return fmt.Errorf("state %s", op.State)
		}
		return nil
	})
	opBefore, _ := h.operation(broken.OperationID)
	var crBefore platform.Application
	h.c[a].Get(ctx, client.ObjectKey{Namespace: "hf-demo", Name: "pricing-a"}, &crBefore)
	h.run("bash", "stages/m3/scripts/control.sh", "m3-hub-stop")
	h.run("bash", "stages/m3/scripts/control.sh", "m3-hub-up")
	h.clusters(map[string]string{a: "CONNECTED", b: "CONNECTED"})
	opAfter, e := h.operation(broken.OperationID)
	if e != nil || opAfter.CommandID != opBefore.CommandID || opAfter.DeadlineAt != opBefore.DeadlineAt {
		t.Fatal("Hub restart lost durable operation", opAfter, e)
	}
	var crAfter platform.Application
	h.c[a].Get(ctx, client.ObjectKey{Namespace: "hf-demo", Name: "pricing-a"}, &crAfter)
	if crAfter.UID != crBefore.UID || crAfter.Generation != crBefore.Generation {
		t.Fatal("Hub restart resent harmful mutation")
	}
	h.terminal(broken, "FAILED")
	three := int64(3)
	rollback := h.accept("POST", path+"/rollback", domain.RollbackRequest{TargetRevision: 2, ExpectedDesiredRevision: &three})
	h.terminal(rollback, "SUCCEEDED")
	h.quote(a, "pricing-a", 18280, "v1")
	h.lateReady(a, b, first.ApplicationID)
	d, e := h.detail(first.ApplicationID)
	if e != nil {
		t.Fatal(e)
	}
	deleted := h.accept("DELETE", path, domain.DeleteRequest{ExpectedDesiredRevision: &d.Application.DesiredRevision})
	h.terminal(deleted, "SUCCEEDED")
	db, e := h.detail(second.ApplicationID)
	if e != nil {
		t.Fatal(e)
	}
	deletedB := h.accept("DELETE", "/api/v1/tenants/demo/apps/"+second.ApplicationID, domain.DeleteRequest{ExpectedDesiredRevision: &db.Application.DesiredRevision})
	h.terminal(deletedB, "SUCCEEDED")
	for _, id := range []string{a, b} {
		var apps platform.ApplicationList
		if e = h.c[id].List(ctx, &apps, client.InNamespace("hf-demo")); e != nil || len(apps.Items) != 0 {
			t.Fatalf("CR garbage collection %s %+v %v", id, apps, e)
		}
	}
	t.Log("PASS two real Worker Pod channels, pricing HTTP, offline repair, reconnect, durable retries, rollback, late-ready and GC")
}

func (h *harness) probeReplay(id string) {
	h.t.Helper()
	ctx := h.ctx
	// Use the actual Agent ServiceAccount, with no kubeconfig credential plugins.
	expiry := int64(600)
	tr, err := h.admin[id].CoreV1().ServiceAccounts("hf-demo").CreateToken(ctx, "hugofleet-agent", &authv1.TokenRequest{Spec: authv1.TokenRequestSpec{ExpirationSeconds: &expiry}}, metav1.CreateOptions{})
	if err != nil {
		h.t.Fatal(err)
	}
	adminCfg, err := clientcmd.BuildConfigFromFlags("", filepath.Join(h.root, ".local/kubeconfig-"+id))
	if err != nil {
		h.t.Fatal(err)
	}
	cfg := &rest.Config{Host: adminCfg.Host, TLSClientConfig: rest.TLSClientConfig{CAData: adminCfg.CAData}, BearerToken: tr.Status.Token, Timeout: 5 * time.Second}
	// Confirm actual negative RBAC, rather than inferring it from YAML.
	auth, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, q := range []struct {
		verb, group, res, sub, ns, name string
		allowed                         bool
	}{{"update", "", "configmaps", "", "hf-demo", agent.LedgerName, true}, {"get", "", "secrets", "", "hf-demo", "", false}, {"create", "", "pods", "exec", "hf-demo", "", false}, {"create", "apps", "deployments", "", "hf-demo", "", false}, {"create", "platform.hugofleet.io", "applications", "", "default", "", false}, {"update", "", "configmaps", "", "hf-demo", "foreign", false}} {
		review, err := auth.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authzv1.SelfSubjectAccessReview{Spec: authzv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authzv1.ResourceAttributes{Verb: q.verb, Group: q.group, Resource: q.res, Subresource: q.sub, Namespace: q.ns, Name: q.name}}}, metav1.CreateOptions{})
		if err != nil || review.Status.Allowed != q.allowed {
			h.t.Fatalf("Agent RBAC %+v %v %v", q, review, err)
		}
	}
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	restart := func() *agent.Engine {
		c, e := client.New(cfg, client.Options{Scheme: scheme})
		if e != nil {
			h.t.Fatal(e)
		}
		work, e := kube.NewWithConfig(cfg)
		if e != nil {
			h.t.Fatal(e)
		}
		return &agent.Engine{Cluster: id, Ledger: &agent.KubeLedger{Client: c}, Workloads: work}
	}
	engine := restart()
	a := domain.Application{ID: uuid.NewString(), TenantID: "demo", Name: "replay-probe", DesiredRevision: 1}
	spec, _ := domain.DefaultPolicy().Spec(domain.DeployRequest{Image: "pricing-api:v1"})
	command := func(rev int64, cmd string) *pb.HubEnvelope {
		a.DesiredRevision = rev
		spec.Revision = rev
		c := &pb.HubEnvelope{CommandId: cmd, OperationId: "probe", ExpiresAt: time.Now().Add(5 * time.Second).UnixMilli(), Payload: &pb.HubEnvelope_Apply{Apply: &pb.ApplyApplication{Application: agentprotocol.Identity(a), Spec: agentprotocol.Spec(spec)}}}
		c.PayloadHash = agentprotocol.PayloadHash(c)
		return c
	}
	first := command(1, "probe-1")
	result := engine.Execute(ctx, first)
	if result.ErrorCode != "" {
		h.t.Fatal(result)
	}
	var cr platform.Application
	if err = h.c[id].Get(ctx, client.ObjectKey{Namespace: "hf-demo", Name: a.Name}, &cr); err != nil {
		h.t.Fatal(err)
	}
	uid, generation := cr.UID, cr.Generation
	engine = restart()
	first.ExpiresAt = time.Now().Add(5 * time.Second).UnixMilli()
	if r := engine.Execute(ctx, first); r.ErrorCode != "" {
		h.t.Fatal(r)
	}
	if err = h.c[id].Get(ctx, client.ObjectKey{Namespace: "hf-demo", Name: a.Name}, &cr); err != nil || cr.UID != uid || cr.Generation != generation {
		h.t.Fatal("lost ACK caused duplicate CR mutation", err)
	}
	second := command(2, "probe-2")
	if r := engine.Execute(ctx, second); r.ErrorCode != "" {
		h.t.Fatal(r)
	}
	engine = restart()
	first.ExpiresAt = time.Now().Add(5 * time.Second).UnixMilli()
	if r := engine.Execute(ctx, first); r.ErrorCode != "STALE_REVISION" {
		h.t.Fatal("stale accepted", r)
	}
	bad := command(2, "probe-different")
	bad.GetApply().Spec.Image = "pricing-api:v2-good"
	bad.PayloadHash = agentprotocol.PayloadHash(bad)
	if r := engine.Execute(ctx, bad); r.ErrorCode != "CONFLICT" {
		h.t.Fatal("same revision different payload accepted", r)
	}
	a.KubernetesUID = string(uid)
	a.DesiredRevision = 2
	del := &pb.HubEnvelope{CommandId: "probe-delete", OperationId: "probe-delete-op", ExpiresAt: time.Now().Add(-time.Second).UnixMilli(), Payload: &pb.HubEnvelope_Delete{Delete: &pb.DeleteApplication{Application: agentprotocol.Identity(a)}}}
	del.PayloadHash = agentprotocol.PayloadHash(del)
	if r := engine.Execute(ctx, del); r.ErrorCode != "EXPIRED" {
		h.t.Fatal("expired delete accepted", r)
	}
	h.c[id].Get(ctx, client.ObjectKey{Namespace: "hf-demo", Name: a.Name}, &cr)
	if cr.UID != uid {
		h.t.Fatal("expired delete removed CR")
	}
	del.ExpiresAt = time.Now().Add(5 * time.Second).UnixMilli()
	if r := engine.Execute(ctx, del); r.ErrorCode != "" {
		h.t.Fatal(r)
	}
	engine = restart()
	h.wait("real GC and durable delete replay", func() error {
		del.ExpiresAt = time.Now().Add(5 * time.Second).UnixMilli()
		r := engine.Execute(ctx, del)
		if r.ErrorCode != "" {
			return fmt.Errorf("delete %s", r.ErrorCode)
		}
		if !r.Status.Deleted {
			return fmt.Errorf("GC pending")
		}
		return nil
	})
	resurrection := command(3, "probe-3")
	if r := engine.Execute(ctx, resurrection); r.ErrorCode != "DELETED" {
		h.t.Fatal("tombstone did not protect deleted CR", r)
	}
	h.t.Log("PASS real restricted SA, ACK loss, Agent restart ledger, stale/conflicting revision, expired delete, GC and tombstone")
}
func (h *harness) lateReady(a, b, app string) {
	h.t.Helper()
	ctx := h.ctx
	// Recreate only the owned Hub container with a 20s deadline; the DB/CA stay.
	original := os.Getenv("HF_M3_OPERATION_TIMEOUT")
	os.Setenv("HF_M3_OPERATION_TIMEOUT", "20s")
	defer os.Setenv("HF_M3_OPERATION_TIMEOUT", original)
	h.run("bash", "stages/m3/scripts/control.sh", "m3-hub-recreate")
	// The new Docker address may differ. Bootstrap resolves it and restarts Agents.
	h.run("bash", "stages/m3/scripts/control.sh", "agent-install")
	h.clusters(map[string]string{a: "CONNECTED", b: "CONNECTED"})
	// Cordon only this test's single owned node. Existing Operator/Agent Pods
	// keep running; new workload Pods remain Pending without FailedCreate.
	nodes, e := h.admin[a].CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if e != nil {
		h.t.Fatal(e)
	}
	if len(nodes.Items) != 1 || nodes.Items[0].Spec.Unschedulable {
		h.t.Fatal("expected one schedulable test node", nodes.Items)
	}
	nodeName := nodes.Items[0].Name
	setCordon := func(ctx context.Context, value bool) error {
		node, err := h.admin[a].CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		node.Spec.Unschedulable = value
		_, err = h.admin[a].CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		return err
	}
	if e = setCordon(ctx, true); e != nil {
		h.t.Fatal(e)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := setCordon(cleanupCtx, false); err != nil {
			h.t.Error("restore test node scheduling", err)
		}
	}()
	d, e := h.detail(app)
	if e != nil {
		h.t.Fatal(e)
	}
	lastSuccessful := d.Application.LastSuccessfulRevision
	accepted := h.accept("POST", "/api/v1/tenants/demo/apps/"+app+"/releases", domain.DeployRequest{Image: "pricing-api:v1", Replicas: 3, ExpectedDesiredRevision: &d.Application.DesiredRevision})
	h.wait("timeout candidate really RUNNING and CR applied", func() error {
		op, e := h.operation(accepted.OperationID)
		if e != nil {
			return e
		}
		d, e := h.detail(app)
		if e != nil {
			return e
		}
		if op.State != "RUNNING" || d.Application.AppliedRevision != accepted.Revision {
			return fmt.Errorf("operation %s applied %d", op.State, d.Application.AppliedRevision)
		}
		return nil
	})
	h.wait("real new Pod Pending because test node is cordoned", func() error {
		pods, err := h.admin[a].CoreV1().Pods("hf-demo").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=pricing-a"})
		if err != nil {
			return err
		}
		for _, p := range pods.Items {
			for _, c := range p.Status.Conditions {
				if p.Status.Phase == corev1.PodPending && c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse && c.Reason == "Unschedulable" {
					return nil
				}
			}
		}
		return fmt.Errorf("waiting for scheduler evidence")
	})
	before, e := h.operation(accepted.OperationID)
	if e != nil {
		h.t.Fatal(e)
	}
	var cr platform.Application
	h.c[a].Get(ctx, client.ObjectKey{Namespace: "hf-demo", Name: "pricing-a"}, &cr)
	uid, generation := cr.UID, cr.Generation
	h.run("bash", "stages/m3/scripts/control.sh", "m3-hub-stop")
	wait := time.Until(time.UnixMilli(before.DeadlineAt).Add(time.Second))
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			h.t.Fatal(ctx.Err())
		case <-timer.C:
		}
	}
	h.run("bash", "stages/m3/scripts/control.sh", "m3-hub-up")
	h.clusters(map[string]string{a: "CONNECTED", b: "CONNECTED"})
	h.terminal(accepted, "TIMED_OUT")
	if e = setCordon(ctx, false); e != nil {
		h.t.Fatal(e)
	}
	h.wait("late Ready is observation while failed history remains", func() error {
		d, e := h.detail(app)
		if e != nil {
			return e
		}
		op, e := h.operation(accepted.OperationID)
		if e != nil {
			return e
		}
		if op.State != "TIMED_OUT" || op.CommandID != before.CommandID || op.DeadlineAt != before.DeadlineAt || d.Application.LastSuccessfulRevision != lastSuccessful {
			h.t.Fatal("late result rewrote durable history", d, op)
		}
		if d.Application.Status != "READY" {
			return fmt.Errorf("status %s", d.Application.Status)
		}
		found := false
		for _, r := range d.Releases {
			if r.Revision == accepted.Revision {
				found = r.State == "FAILED"
			}
		}
		if !found {
			h.t.Fatal("late ready rewrote failed release")
		}
		h.c[a].Get(ctx, client.ObjectKey{Namespace: "hf-demo", Name: "pricing-a"}, &cr)
		if cr.UID != uid || cr.Generation != generation {
			h.t.Fatal("restart rewrote timed-out CR")
		}
		return nil
	})
}
