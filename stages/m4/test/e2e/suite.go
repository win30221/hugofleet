//go:build e2e

package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m4/bootstrap"
	"github.com/win30221/hugofleet/stages/m4/testdriver"
	"io"
	appsv1 "k8s.io/api/apps/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
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

type worker struct {
	client.Client
	Typed          *kubernetes.Clientset
	ID, Kubeconfig string
}
type suite struct {
	stage    string
	t        *testing.T
	ctx      context.Context
	sessions map[string]*testdriver.ManagementSession
	workers  map[string]worker
	A, B     string
}

func (s *suite) api(who, method, path, key string, body any, want int, out any) []byte {
	s.t.Helper()
	status, raw, err := s.sessions[who].Request(s.ctx, method, path, key, body)
	if err != nil || status != want {
		var problem struct {
			Error struct {
				Code, Message string
				RequestID     string `json:"request_id"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &problem)
		s.t.Fatalf("%s %s %s expected %d got %d: code=%s reason=%s request_id=%s (%v)", who, method, path, want, status, problem.Error.Code, problem.Error.Message, problem.Error.RequestID, err)
	}
	if out != nil {
		if err = json.Unmarshal(raw, out); err != nil {
			s.t.Fatal(err)
		}
	}
	return raw
}
func eventually(t *testing.T, ctx context.Context, timeout time.Duration, description string, f func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		if f() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatalf("timed out: %s", description)
		case <-tick.C:
		}
	}
}

// RunRealTenantSecurity reuses the same actual Provider/HTTP/Kubernetes/packet
// assertions for M5. Only the explicit stage identity and deployment owner differ.
func RunRealTenantSecurity(t *testing.T, stage string) {
	if stage != "m4" && stage != "m5" {
		t.Fatal("explicit m4 or m5 security acceptance required")
	}
	prefix := "HF_" + strings.ToUpper(stage)
	state := os.Getenv(prefix + "_STATE")
	a, b := os.Getenv(prefix+"_A"), os.Getenv(prefix+"_B")
	if a == "" || b == "" || state == "" {
		t.Fatal("real owned M4 environment required")
	}
	settings, err := testdriver.ReadSettings(filepath.Join(state, "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	s := &suite{t: t, ctx: ctx, A: a, B: b, sessions: map[string]*testdriver.ManagementSession{}, workers: map[string]worker{}, stage: stage}
	hub := os.Getenv(prefix + "_HUB")
	if hub != "hugofleet-hub-"+stage+"-e2e" && hub != "hugofleet-hub-"+stage+"-development" {
		t.Fatal("exact owned M4 E2E Hub required")
	}
	inspectCtx, inspectCancel := context.WithTimeout(ctx, 10*time.Second)
	mountRaw, err := exec.CommandContext(inspectCtx, "docker", "inspect", "--format", "{{json .Mounts}}", hub).Output()
	inspectCancel()
	var mounts []struct {
		Type, Source, Destination string
		RW                        bool
	}
	if err != nil || json.Unmarshal(mountRaw, &mounts) != nil || len(mounts) != 5 {
		t.Fatal("actual Hub isolated mount inventory invalid")
	}
	allowed := map[string]string{"/data": "db", "/config.json": "hub-config.json", "/certs/ca.crt": "ca.crt", "/certs/hub.crt": "hub.crt", "/certs/hub.key": "hub.key"}
	for _, mount := range mounts {
		if mount.Type != "bind" || allowed[mount.Destination] != filepath.Base(mount.Source) || (mount.Destination != "/data" && mount.RW) {
			t.Fatal("actual Hub mounted a Worker credential, kubeconfig, Docker socket or unexpected writable path")
		}
	}
	for _, account := range settings.Accounts {
		v, err := testdriver.LoginHub(ctx, settings.OIDC, account)
		if err != nil {
			t.Fatalf("actual login %s: %v", account.Name, err)
		}
		s.sessions[account.Name] = v
		if v.User.PlatformAdmin != (account.Name == "admin") {
			t.Fatal("OIDC claim escalated role")
		}
	}
	// Exercise the real middleware with a verified session before any mutation.
	// A failed assertion stops the suite; no security response is simulated.
	admin := s.sessions["admin"]
	for _, negative := range []string{"missing-csrf", "wrong-csrf", "missing-origin", "wrong-origin", "cross-site"} {
		r, err := http.NewRequestWithContext(ctx, "POST", admin.Config.BaseURL+"/logout", nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", admin.Config.BaseURL)
		r.Header.Set("X-CSRF-Token", admin.CSRF)
		switch negative {
		case "missing-csrf":
			r.Header.Del("X-CSRF-Token")
		case "wrong-csrf":
			r.Header.Set("X-CSRF-Token", "invalid")
		case "missing-origin":
			r.Header.Del("Origin")
		case "wrong-origin":
			r.Header.Set("Origin", "https://untrusted.example.test")
		case "cross-site":
			r.Header.Set("Sec-Fetch-Site", "cross-site")
		}
		response, err := admin.Client.Do(r)
		if err != nil {
			t.Fatal("real CSRF negative request failed")
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("real %s request accepted: HTTP %d", negative, response.StatusCode)
		}
	}
	s.api("admin", "GET", "/api/v1/auth/me", "", nil, 200, nil)
	t.Log("PASS real session middleware rejects missing/wrong CSRF and Origin, cross-site mutation; session remains valid")
	for _, id := range []string{a, b} {
		kubeconfig := filepath.Join(state, "clusters", id, "kubeconfig")
		raw, err := clientcmd.LoadFromFile(kubeconfig)
		if err != nil || raw.CurrentContext != "kind-"+id {
			t.Fatal("exact owned kind context required")
		}
		cfg, err := clientcmd.NewNonInteractiveClientConfig(*raw, "kind-"+id, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
		if err != nil {
			t.Fatal(err)
		}
		cfg.Timeout = 10 * time.Second
		scheme := runtime.NewScheme()
		for _, add := range []func(*runtime.Scheme) error{platform.AddToScheme, appsv1.AddToScheme, corev1.AddToScheme, networkingv1.AddToScheme, authzv1.AddToScheme} {
			if err = add(scheme); err != nil {
				t.Fatal(err)
			}
		}
		c, err := client.New(cfg, client.Options{Scheme: scheme})
		if err != nil {
			t.Fatal(err)
		}
		typed, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}
		s.workers[id] = worker{c, typed, id, kubeconfig}
	}
	var clusters []domain.Cluster
	s.api("admin", "GET", "/api/v1/clusters", "", nil, 200, &clusters)
	if len(clusters) != 2 {
		t.Fatal("real dual Worker registry incomplete")
	}
	for _, c := range clusters {
		if c.State != "CONNECTED" || c.AgentVersion != stage || c.LastSeenAt == 0 {
			t.Fatalf("Worker not really connected: %+v", c)
		}
	}
	for _, tenant := range []string{"team-a", "team-b"} {
		owner := "owner-a"
		if tenant == "team-b" {
			owner = "owner-b"
		}
		for _, id := range []string{a, b} {
			s.api("admin", "POST", "/api/v1/tenants", uuid.NewString(), map[string]string{"slug": tenant, "name": tenant, "cluster_id": id, "owner_id": s.sessions[owner].User.ID}, 201, nil)
		}
	}
	for who, role := range map[string]string{"developer-a": "Developer", "viewer-a": "Viewer"} {
		s.api("owner-a", "POST", "/api/v1/tenants/team-a/members", "", map[string]string{"user_id": s.sessions[who].User.ID, "role": role}, 200, nil)
	}
	s.api("owner-a", "POST", "/api/v1/tenants/team-a/members", "", map[string]string{"user_id": s.sessions["owner-a"].User.ID, "role": "Viewer"}, 409, nil)
	zero := int64(0)
	create := domain.DeployRequest{Name: "pricing-api", ClusterID: a, ExpectedDesiredRevision: &zero, Image: "pricing-api:v1"}
	key := uuid.NewString()
	var accepted domain.Accepted
	s.api("developer-a", "POST", "/api/v1/tenants/team-a/apps", key, create, 202, &accepted)
	s.wait("developer-a", "team-a", accepted, "SUCCEEDED", 1)
	var replay domain.Accepted
	s.api("developer-a", "POST", "/api/v1/tenants/team-a/apps", key, create, 202, &replay)
	if replay.OperationID != accepted.OperationID {
		t.Fatal("idempotency replay created another operation")
	}
	var other domain.Accepted
	s.api("owner-b", "POST", "/api/v1/tenants/team-b/apps", uuid.NewString(), create, 202, &other)
	s.wait("owner-b", "team-b", other, "SUCCEEDED", 1)
	createB := create
	createB.Name = "pricing-b"
	createB.ClusterID = b
	var remote domain.Accepted
	s.api("owner-b", "POST", "/api/v1/tenants/team-b/apps", uuid.NewString(), createB, 202, &remote)
	s.wait("owner-b", "team-b", remote, "SUCCEEDED", 1)
	base := "/api/v1/tenants/team-b/apps"
	one := int64(1)
	for _, path := range []string{base, base + "/" + other.ApplicationID, "/api/v1/operations/" + other.OperationID, "/api/v1/audit?tenant=team-b", "/api/v1/tenants/team-b/quota", "/api/v1/tenants/team-b/members"} {
		s.api("developer-a", "GET", path, "", nil, 403, nil)
	}
	s.api("developer-a", "POST", base, uuid.NewString(), create, 403, nil)
	change := domain.DeployRequest{ExpectedDesiredRevision: &one, Image: "pricing-api:v2-good"}
	s.api("developer-a", "POST", base+"/"+other.ApplicationID+"/releases", uuid.NewString(), change, 403, nil)
	s.api("developer-a", "POST", base+"/"+other.ApplicationID+"/rollback", uuid.NewString(), domain.RollbackRequest{ExpectedDesiredRevision: &one, TargetRevision: 1}, 403, nil)
	s.api("developer-a", "DELETE", "/api/v1/tenants/team-a/apps/"+accepted.ApplicationID, uuid.NewString(), domain.DeleteRequest{ExpectedDesiredRevision: &one}, 403, nil)
	s.api("viewer-a", "POST", "/api/v1/tenants/team-a/apps", uuid.NewString(), create, 403, nil)
	s.api("outsider", "GET", "/api/v1/tenants/team-a/apps", "", nil, 403, nil)
	s.api("developer-a", "GET", "/api/v1/users", "", nil, 403, nil)
	s.api("owner-a", "POST", "/api/v1/tenants/team-a/members", "", map[string]string{"user_id": s.sessions["developer-a"].User.ID, "role": ""}, 200, nil)
	s.api("developer-a", "POST", "/api/v1/tenants/team-a/apps", key, create, 403, nil)
	s.api("owner-a", "POST", "/api/v1/tenants/team-a/members", "", map[string]string{"user_id": s.sessions["developer-a"].User.ID, "role": "Developer"}, 200, nil)
	var audit []domain.AuditEvent
	s.api("owner-a", "GET", "/api/v1/audit?tenant=team-a&limit=100", "", nil, 200, &audit)
	found := false
	for _, event := range audit {
		if event.Action == "CREATE" && event.Outcome == "ACCEPTED" && event.ActorID == s.sessions["developer-a"].User.ID && event.RequestID != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("authenticated intent audit absent")
	}
	var samplePods corev1.PodList
	if err := s.workers[a].List(ctx, &samplePods, client.InNamespace("hf-team-a"), client.MatchingLabels{"app.kubernetes.io/name": "pricing-api"}); err != nil || len(samplePods.Items) == 0 {
		t.Fatal("actual sample Pod missing")
	}
	for _, pod := range samplePods.Items {
		if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
			t.Fatal("sample app mounted Kubernetes service account token")
		}
	}
	viewer := s.api("viewer-a", "GET", "/tenants/team-a", "", nil, 200, nil)
	if bytes.Contains(viewer, []byte(`action="/ui/tenants/team-a/apps"`)) || !bytes.Contains(viewer, []byte("Viewer")) {
		t.Fatal("Viewer HTML exposed mutation form")
	}
	s.quote(s.workers[a], "hf-team-a", "pricing-api", 18085)
	s.quote(s.workers[b], "hf-team-b", "pricing-b", 18086)
	dep := &appsv1.Deployment{}
	w := s.workers[a]
	if err := w.Get(ctx, client.ObjectKey{Namespace: "hf-team-a", Name: "pricing-api"}, dep); err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Fatal("initial real Deployment replicas incorrect")
	}
	dep.Spec.Replicas = p(int32(2))
	if err := w.Update(ctx, dep); err != nil {
		t.Fatal(err)
	}
	eventually(t, ctx, time.Minute, "namespaced Operator repairs actual Deployment drift", func() bool {
		if err := w.Get(ctx, client.ObjectKeyFromObject(dep), dep); err != nil {
			t.Fatal(err)
		}
		return dep.Spec.Replicas != nil && *dep.Spec.Replicas == 1 && dep.Status.ObservedGeneration == dep.Generation && dep.Status.Replicas == 1 && dep.Status.ReadyReplicas == 1
	})
	s.rbac()
	t.Log("PASS real OIDC/SQLite/mTLS dual Worker: scoped read/write/rollback/operation/audit, Viewer, revoke-before-replay, trusted actor, HTTP business result and SA permissions")
	s.network()
	s.quota("cpu")
	s.quota("memory")
	t.Log("PASS M4 real tenant isolation, CPU/memory Admission and enforcing NetworkPolicy")
	s.lesson()
	s.recreatePreservesPolicy()
}
func (s *suite) wait(who, tenant string, a domain.Accepted, state string, rev int64) domain.Detail {
	s.t.Helper()
	var detail domain.Detail
	eventually(s.t, s.ctx, 4*time.Minute, "real operation "+state, func() bool {
		var op domain.Operation
		s.api(who, "GET", "/api/v1/operations/"+a.OperationID, "", nil, 200, &op)
		if op.Active() {
			return false
		}
		if op.State != state {
			s.t.Fatalf("operation %s expected %s got %+v", a.OperationID, state, op)
		}
		s.api(who, "GET", "/api/v1/tenants/"+tenant+"/apps/"+a.ApplicationID, "", nil, 200, &detail)
		if detail.Application.DesiredRevision != rev || detail.Application.AppliedRevision != rev || detail.Application.KubernetesUID == "" {
			return false
		}
		if state == "SUCCEEDED" && (detail.Application.Status != "READY" || detail.Application.LastSuccessfulRevision != rev) {
			return false
		}
		return true
	})
	return detail
}
func (s *suite) quote(w worker, ns, name string, port int) {
	s.t.Helper()
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", w.Kubeconfig, "--context", "kind-"+w.ID, "-n", ns, "port-forward", "service/"+name, fmt.Sprintf("%d:8080", port), "--address=127.0.0.1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout
	if err = cmd.Start(); err != nil {
		s.t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	ready := make(chan bool, 1)
	go func() {
		scan := bufio.NewScanner(stdout)
		for scan.Scan() {
			if strings.Contains(scan.Text(), fmt.Sprintf("Forwarding from 127.0.0.1:%d", port)) {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			s.t.Fatal("owned service port-forward could not bind")
		}
	case <-time.After(20 * time.Second):
		s.t.Fatal("service forward deadline")
	}
	c := &http.Client{Timeout: 5 * time.Second}
	r, err := c.Post(fmt.Sprintf("http://127.0.0.1:%d/quote", port), "application/json", strings.NewReader(`{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}`))
	if err != nil {
		s.t.Fatal(err)
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	var quote map[string]int64
	if err != nil || r.StatusCode != 200 || json.Unmarshal(raw, &quote) != nil || quote["original_amount_cents"] != 20000 || quote["tier_discount_cents"] != 2000 || quote["coupon_discount_cents"] != 10000 || quote["final_amount_cents"] != 8000 {
		s.t.Fatalf("actual pricing business response HTTP %d %s", r.StatusCode, raw)
	}
}
func (s *suite) rbac() {
	for _, w := range s.workers {
		for _, item := range []struct {
			user, namespace, group, resource, subresource, verb, name string
			want                                                      bool
		}{
			{"system:serviceaccount:hf-system:hugofleet-agent", "hf-team-a", "platform.hugofleet.io", "applications", "", "create", "", true},
			{"system:serviceaccount:hf-system:hugofleet-agent", "hf-team-b", "platform.hugofleet.io", "applications", "", "create", "", true},
			{"system:serviceaccount:hf-system:hugofleet-agent", "default", "platform.hugofleet.io", "applications", "", "create", "", false},
			{"system:serviceaccount:hf-system:hugofleet-agent", "hf-team-a", "", "secrets", "", "get", "", false},
			{"system:serviceaccount:hf-system:hugofleet-agent", "hf-system", "", "secrets", "", "get", "hugofleet-agent-tls", false},
			{"system:serviceaccount:hf-system:hugofleet-agent", "hf-team-a", "", "pods", "exec", "create", "", false},
			{"system:serviceaccount:hf-system:hugofleet-agent", "", "", "namespaces", "", "create", "", false},
			{"system:serviceaccount:hf-system:hugofleet-agent", "hf-team-a", "rbac.authorization.k8s.io", "roles", "", "update", "", false},
			{"system:serviceaccount:hf-system:hugofleet-agent", "hf-team-a", "", "resourcequotas", "", "update", bootstrap.QuotaName, false},
			{"system:serviceaccount:hf-system:hugofleet-agent", "hf-team-a", "", "configmaps", "", "update", bootstrap.ReceiptName, false},
			{"system:serviceaccount:hf-system:hugofleet-agent", "hf-team-a", "", "configmaps", "", "get", bootstrap.ReceiptName, true},
			{"system:serviceaccount:hf-team-a:hugofleet-operator", "hf-team-a", "apps", "deployments", "", "create", "", true},
			{"system:serviceaccount:hf-team-a:hugofleet-operator", "hf-team-a", "apps", "replicasets", "", "list", "", true},
			{"system:serviceaccount:hf-team-a:hugofleet-operator", "hf-team-a", "apps", "replicasets", "", "watch", "", true},
			{"system:serviceaccount:hf-team-a:hugofleet-operator", "hf-team-a", "apps", "replicasets", "", "update", "", false},
			{"system:serviceaccount:hf-team-a:hugofleet-operator", "hf-team-b", "apps", "replicasets", "", "list", "", false},
			{"system:serviceaccount:hf-team-a:hugofleet-operator", "hf-team-b", "apps", "deployments", "", "create", "", false},
			{"system:serviceaccount:hf-team-a:hugofleet-operator", "hf-team-a", "", "secrets", "", "get", "", false},
			{"system:serviceaccount:hf-team-a:default", "hf-team-a", "platform.hugofleet.io", "applications", "", "get", "", false},
		} {
			sar := &authzv1.SubjectAccessReview{Spec: authzv1.SubjectAccessReviewSpec{User: item.user, ResourceAttributes: &authzv1.ResourceAttributes{Namespace: item.namespace, Group: item.group, Resource: item.resource, Subresource: item.subresource, Verb: item.verb, Name: item.name}}}
			if err := w.Create(s.ctx, sar); err != nil {
				s.t.Fatal(err)
			}
			if sar.Status.Allowed != item.want {
				s.t.Fatalf("SA permission wrong Worker=%s %+v: %+v", w.ID, item, sar.Status)
			}
		}
	}
}
func p[T any](v T) *T { return &v }
func testPod(ns, name string, args []string, rcpu, rmem, lcpu, lmem string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"app.kubernetes.io/managed-by": "hugofleet-m4-e2e"}}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, AutomountServiceAccountToken: p(false), TerminationGracePeriodSeconds: p(int64(0)), SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: p(true), RunAsUser: p(int64(65532)), RunAsGroup: p(int64(65532)), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, Containers: []corev1.Container{{Name: "probe", Image: "hugofleet-netprobe:m4", ImagePullPolicy: corev1.PullIfNotPresent, Args: args, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(rcpu), corev1.ResourceMemory: resource.MustParse(rmem)}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(lcpu), corev1.ResourceMemory: resource.MustParse(lmem)}}, SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: p(true), AllowPrivilegeEscalation: p(false), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}}}}}
}
func (s *suite) deletePods(w worker, pods ...*corev1.Pod) {
	for _, pod := range pods {
		err := w.Delete(s.ctx, pod)
		if err != nil && !apierrors.IsNotFound(err) {
			s.t.Fatal(err)
		}
	}
	for _, pod := range pods {
		eventually(s.t, s.ctx, time.Minute, "owned Pod deletion", func() bool { return apierrors.IsNotFound(w.Get(s.ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{})) })
	}
}
func (s *suite) runProbe(w worker, ns, target, expect string) {
	pod := testPod(ns, "hf-probe-"+uuid.NewString()[:8], []string{"--target=" + target, "--expect=" + expect}, "10m", "16Mi", "100m", "32Mi")
	if err := w.Create(s.ctx, pod); err != nil {
		s.t.Fatal(err)
	}
	defer s.deletePods(w, pod)
	eventually(s.t, s.ctx, 2*time.Minute, "actual network probe completion", func() bool {
		if err := w.Get(s.ctx, client.ObjectKeyFromObject(pod), pod); err != nil {
			s.t.Fatal(err)
		}
		return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
	})
	raw, err := w.Typed.CoreV1().Pods(ns).GetLogs(pod.Name, &corev1.PodLogOptions{}).DoRaw(s.ctx)
	if err != nil {
		s.t.Fatal(err)
	}
	if pod.Status.Phase != corev1.PodSucceeded || !bytes.Contains(raw, []byte("PASS actual Pod HTTP probe: "+expect)) {
		s.t.Fatalf("network probe expected %s got %s: %s", expect, pod.Status.Phase, raw)
	}
	s.t.Logf("Worker %s source %s: %s", w.ID, ns, strings.TrimSpace(string(raw)))
}
func (s *suite) network() {
	w := s.workers[s.A]
	ns := "hf-team-b"
	policy := &networkingv1.NetworkPolicy{}
	if err := w.Get(s.ctx, client.ObjectKey{Namespace: ns, Name: "hugofleet-tenant-ingress"}, policy); err != nil {
		s.t.Fatal(err)
	}
	if policy.Labels[bootstrap.ManagedLabel] != bootstrap.Manager || policy.Labels[bootstrap.ClusterLabel] != w.ID {
		s.t.Fatal("foreign policy cannot be modified by acceptance test")
	}
	server := testPod(ns, "hf-network-server", []string{"--serve"}, "30m", "32Mi", "60m", "64Mi")
	server.Spec.Containers[0].ReadinessProbe = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt32(8080)}}, PeriodSeconds: 1}
	if err := w.Create(s.ctx, server); err != nil {
		s.t.Fatal(err)
	}
	defer s.deletePods(w, server)
	eventually(s.t, s.ctx, time.Minute, "real target server running", func() bool {
		if err := w.Get(s.ctx, client.ObjectKeyFromObject(server), server); err != nil {
			s.t.Fatal(err)
		}
		for _, condition := range server.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return server.Status.Phase == corev1.PodRunning && server.Status.PodIP != ""
			}
		}
		return false
	})
	target := "http://" + server.Status.PodIP + ":8080/healthz"
	s.runProbe(w, ns, target, "allowed")
	if err := w.Delete(s.ctx, policy); err != nil {
		s.t.Fatal(err)
	}
	policy.ResourceVersion = ""
	policy.UID = ""
	policy.CreationTimestamp = metav1.Time{}
	restored := false
	defer func() {
		if !restored {
			if err := w.Create(context.Background(), policy); err != nil {
				s.t.Errorf("restore owned ingress policy: %v", err)
			}
		}
	}()
	s.runProbe(w, "hf-team-a", target, "allowed")
	if err := w.Create(s.ctx, policy); err != nil {
		s.t.Fatal(err)
	}
	restored = true
	s.runProbe(w, ns, target, "allowed")
	s.runProbe(w, "hf-team-a", target, "blocked")
	if err := w.Delete(s.ctx, policy); err != nil {
		s.t.Fatal(err)
	}
	restored = false
	policy.ResourceVersion = ""
	policy.UID = ""
	policy.CreationTimestamp = metav1.Time{}
	s.runProbe(w, "hf-team-a", target, "allowed")
	if err := w.Create(s.ctx, policy); err != nil {
		s.t.Fatal(err)
	}
	restored = true
	s.t.Log("PASS actual Calico packets: same-tenant allowed, cross-tenant positive before/after and three policy timeouts with enforcement")
}
func (s *suite) quota(kind string) {
	w := s.workers[s.A]
	ns := "hf-team-a"
	before := &corev1.ResourceQuota{}
	if err := w.Get(s.ctx, client.ObjectKey{Namespace: ns, Name: bootstrap.QuotaName}, before); err != nil {
		s.t.Fatal(err)
	}
	eventually(s.t, s.ctx, time.Minute, "baseline quota usage after previous owned workload GC", func() bool {
		if err := w.Get(s.ctx, client.ObjectKeyFromObject(before), before); err != nil {
			s.t.Fatal(err)
		}
		cpu, mem, pods := before.Status.Used[corev1.ResourceRequestsCPU], before.Status.Used[corev1.ResourceRequestsMemory], before.Status.Used[corev1.ResourcePods]
		return cpu.Cmp(resource.MustParse("200m")) == 0 && mem.Cmp(resource.MustParse("160Mi")) == 0 && pods.Cmp(resource.MustParse("2")) == 0
	})
	pods := []*corev1.Pod{}
	for i := 0; i < 3; i++ {
		cpu, mem := "250m", "16Mi"
		if kind == "memory" {
			cpu, mem = "10m", "250Mi"
		}
		pod := testPod(ns, fmt.Sprintf("hf-quota-%s-%d", kind, i), []string{"--serve"}, cpu, mem, cpu, mem)
		if err := w.Create(s.ctx, pod); err != nil {
			s.t.Fatal(err)
		}
		pods = append(pods, pod)
	}
	removed := false
	defer func() {
		if !removed {
			s.deletePods(w, pods...)
		}
	}()
	key := corev1.ResourceRequestsCPU
	expected := resource.MustParse("950m")
	if kind == "memory" {
		key = corev1.ResourceRequestsMemory
		expected = resource.MustParse("910Mi")
	}
	eventually(s.t, s.ctx, time.Minute, "real quota usage includes load Pods", func() bool {
		q := &corev1.ResourceQuota{}
		if err := w.Get(s.ctx, client.ObjectKeyFromObject(before), q); err != nil {
			s.t.Fatal(err)
		}
		v := q.Status.Used[key]
		return v.Cmp(expected) == 0
	})
	zero := int64(0)
	req := domain.DeployRequest{Name: "quota-" + kind, ClusterID: s.A, ExpectedDesiredRevision: &zero, Image: "pricing-api:v1"}
	if kind == "memory" {
		req.Resources = platform.ApplicationResources{Requests: platform.ResourceValues{CPU: "100m", Memory: "128Mi"}, Limits: platform.ResourceValues{CPU: "300m", Memory: "256Mi"}}
	}
	var a domain.Accepted
	s.api("developer-a", "POST", "/api/v1/tenants/team-a/apps", uuid.NewString(), req, 202, &a)
	d := s.wait("developer-a", "team-a", a, "FAILED", 1)
	if d.Application.LastSuccessfulRevision != 0 || len(d.Releases) != 1 || d.Releases[0].State != "FAILED" {
		s.t.Fatal("quota failure manufactured success or lost immutable failed release")
	}
	var op domain.Operation
	s.api("developer-a", "GET", "/api/v1/operations/"+a.OperationID, "", nil, 200, &op)
	if !strings.Contains(strings.ToLower(op.ErrorMessage), "exceeded quota") || !strings.Contains(op.ErrorMessage, string(key)) {
		s.t.Fatalf("Kubernetes %s Admission cause not propagated: %+v", kind, op)
	}
	deployment := &appsv1.Deployment{}
	if err := w.Get(s.ctx, client.ObjectKey{Namespace: ns, Name: req.Name}, deployment); err != nil {
		s.t.Fatal(err)
	}
	replicaFailure := false
	for _, c := range deployment.Status.Conditions {
		if c.Type == appsv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue && strings.Contains(c.Message, "exceeded quota") {
			replicaFailure = true
		}
	}
	if !replicaFailure {
		s.t.Fatal("actual Deployment ReplicaFailure missing")
	}
	var sets appsv1.ReplicaSetList
	if err := w.List(s.ctx, &sets, client.InNamespace(ns)); err != nil {
		s.t.Fatal(err)
	}
	failedCreate := false
	for _, rs := range sets.Items {
		for _, owner := range rs.OwnerReferences {
			if owner.UID == deployment.UID {
				for _, c := range rs.Status.Conditions {
					if c.Reason == "FailedCreate" && strings.Contains(c.Message, "exceeded quota") {
						failedCreate = true
					}
				}
			}
		}
	}
	if !failedCreate {
		s.t.Fatal("actual ReplicaSet FailedCreate missing")
	}
	cr := &platform.Application{}
	if err := w.Get(s.ctx, client.ObjectKey{Namespace: ns, Name: req.Name}, cr); err != nil {
		s.t.Fatal(err)
	}
	degraded := false
	for _, c := range cr.Status.Conditions {
		if c.Type == "Degraded" && c.Status == metav1.ConditionTrue && strings.Contains(c.Message, "exceeded quota") {
			degraded = true
		}
	}
	if !degraded || cr.Status.ObservedGeneration != cr.Generation {
		s.t.Fatal("actual CR Degraded/current generation Admission cause missing")
	}
	ui := s.api("developer-a", "GET", "/tenants/team-a/apps/"+a.ApplicationID, "", nil, 200, nil)
	if !bytes.Contains(ui, []byte("exceeded quota")) || !bytes.Contains(ui, []byte(op.RequestID)) || !bytes.Contains(ui, []byte("FAILED")) {
		s.t.Fatal("real quota reason/request_id/failed history absent from HTML")
	}
	s.deletePods(w, pods...)
	removed = true
	baseline := before.Status.Used[key]
	latePod := baseline.DeepCopy()
	lateAmount := "100m"
	if kind == "memory" {
		lateAmount = "128Mi"
	}
	latePod.Add(resource.MustParse(lateAmount))
	eventually(s.t, s.ctx, time.Minute, "quota usage released before new revision", func() bool {
		q := &corev1.ResourceQuota{}
		if err := w.Get(s.ctx, client.ObjectKeyFromObject(before), q); err != nil {
			s.t.Fatal(err)
		}
		v := q.Status.Used[key]
		return v.Cmp(baseline) <= 0 || v.Cmp(latePod) == 0
	})
	// Admission can later allow the failed revision, but its terminal history may
	// never become success. A fresh revision is required to confirm recovery.
	s.api("developer-a", "GET", "/api/v1/operations/"+a.OperationID, "", nil, 200, &op)
	if op.State != "FAILED" {
		s.t.Fatal("late admission rewrote terminal failed operation")
	}
	one := int64(1)
	var recovered domain.Accepted
	s.api("developer-a", "POST", "/api/v1/tenants/team-a/apps/"+a.ApplicationID+"/releases", uuid.NewString(), domain.DeployRequest{ExpectedDesiredRevision: &one, Image: "pricing-api:v1"}, 202, &recovered)
	s.wait("developer-a", "team-a", recovered, "SUCCEEDED", 2)
	var deleted domain.Accepted
	two := int64(2)
	s.api("owner-a", "DELETE", "/api/v1/tenants/team-a/apps/"+a.ApplicationID, uuid.NewString(), domain.DeleteRequest{ExpectedDesiredRevision: &two}, 202, &deleted)
	eventually(s.t, s.ctx, time.Minute, "actual quota test GC", func() bool {
		var o domain.Operation
		s.api("owner-a", "GET", "/api/v1/operations/"+deleted.OperationID, "", nil, 200, &o)
		return o.State == "SUCCEEDED"
	})
	s.t.Logf("PASS %s actual Pod Admission → ReplicaSet FailedCreate → Deployment ReplicaFailure → CR → Hub FAILED → UI reason/request_id; new revision recovery and GC", kind)
}
