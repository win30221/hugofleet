//go:build e2e

package m2e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"golang.org/x/net/html"
	"io"
	appsv1 "k8s.io/api/apps/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	_ "modernc.org/sqlite"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const base = "http://127.0.0.1:18190"

type harness struct {
	t               *testing.T
	ctx             context.Context
	root, artifacts string
	http            *http.Client
	c               client.Client
	admin           *kubernetes.Clientset
	hub             *exec.Cmd
	hubDone         chan error
	hubLog          *os.File
	starts          int
}

func (h *harness) wait(label string, check func() error) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(h.ctx, 180*time.Second)
	defer cancel()
	var last error
	for {
		last = check()
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
	var e error
	if payload != nil {
		b, e = json.Marshal(payload)
		if e != nil {
			return 0, nil, e
		}
	}
	r, e := http.NewRequestWithContext(h.ctx, method, base+path, bytes.NewReader(b))
	if e != nil {
		return 0, nil, e
	}
	if payload != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	res, e := h.http.Do(r)
	if e != nil {
		return 0, nil, e
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.Header.Get("X-Request-ID") == "" {
		return 0, nil, fmt.Errorf("missing request ID")
	}
	return res.StatusCode, raw, e
}
func (h *harness) accepted(path, key string, payload any) domain.Accepted {
	h.t.Helper()
	status, raw, e := h.request("POST", path, key, payload)
	if e != nil || status != 202 {
		h.t.Fatalf("accept %s: %d %s %v", path, status, raw, e)
	}
	var v domain.Accepted
	if e = json.Unmarshal(raw, &v); e != nil {
		h.t.Fatal(e)
	}
	return v
}
func (h *harness) operation(id string) (domain.Operation, error) {
	status, raw, e := h.request("GET", "/api/v1/operations/"+id, "", nil)
	var v domain.Operation
	if e != nil {
		return v, e
	}
	if status != 200 {
		return v, fmt.Errorf("operation status %d %s", status, raw)
	}
	e = json.Unmarshal(raw, &v)
	return v, e
}
func (h *harness) detail(id string) (domain.Detail, error) {
	status, raw, e := h.request("GET", "/api/v1/tenants/demo/apps/"+id, "", nil)
	var d domain.Detail
	if e != nil {
		return d, e
	}
	if status != 200 {
		return d, fmt.Errorf("detail status %d %s", status, raw)
	}
	e = json.Unmarshal(raw, &d)
	return d, e
}
func (h *harness) terminal(o domain.Accepted, want string) domain.Operation {
	h.t.Helper()
	var op domain.Operation
	h.wait("operation "+want, func() error {
		v, e := h.operation(o.OperationID)
		if e != nil {
			return e
		}
		op = v
		if v.State == want {
			return nil
		}
		if !v.Active() {
			h.t.Fatalf("unexpected terminal operation %+v", v)
		}
		return fmt.Errorf("state=%s", v.State)
	})
	return op
}
func (h *harness) start(timeout string) {
	h.t.Helper()
	h.starts++
	f, e := os.Create(filepath.Join(h.artifacts, fmt.Sprintf("hub-%d.log", h.starts)))
	if e != nil {
		h.t.Fatal(e)
	}
	h.hubLog = f
	h.hub = exec.Command(filepath.Join(h.root, "bin/hub"), "--listen", "127.0.0.1:18190", "--database", filepath.Join(h.artifacts, "hub.db"), "--kubeconfig", os.Getenv("HF_HUB_KUBECONFIG"), "--policy", filepath.Join(h.root, "stages/m2/policy.json"), "--operation-timeout", timeout, "--poll-interval", "500ms")
	h.hub.Dir = h.root
	h.hub.Stdout = f
	h.hub.Stderr = f
	if e = h.hub.Start(); e != nil {
		f.Close()
		h.t.Fatal(e)
	}
	cmd := h.hub
	h.hubDone = make(chan error, 1)
	go func() { h.hubDone <- cmd.Wait() }()
	h.wait("Hub ready and cluster CONNECTED", func() error {
		select {
		case exitErr := <-h.hubDone:
			h.hubDone <- exitErr
			h.t.Fatalf("Hub exited before readiness: %v; inspect hub-%d.log", exitErr, h.starts)
		default:
		}
		status, _, e := h.request("GET", "/readyz", "", nil)
		if e != nil {
			return e
		}
		if status != 200 {
			return fmt.Errorf("readyz=%d", status)
		}
		status, raw, e := h.request("GET", "/api/v1/clusters", "", nil)
		if e != nil {
			return e
		}
		if status != 200 || !strings.Contains(string(raw), "CONNECTED") {
			return fmt.Errorf("cluster not connected")
		}
		return nil
	})
}
func (h *harness) stop(crash bool) {
	h.t.Helper()
	if h.hub == nil {
		return
	}
	if crash {
		h.hub.Process.Kill()
	} else {
		h.hub.Process.Signal(syscall.SIGTERM)
	}
	select {
	case e := <-h.hubDone:
		if !crash && e != nil {
			h.t.Errorf("graceful Hub exit: %v", e)
		}
	case <-time.After(15 * time.Second):
		h.hub.Process.Kill()
		<-h.hubDone
		h.t.Error("Hub shutdown timed out")
	}
	h.hubLog.Close()
	h.hub = nil
}
func (h *harness) operator(replicas int32) {
	h.t.Helper()
	key := client.ObjectKey{Namespace: domain.Namespace, Name: "hugofleet-operator"}
	var dep appsv1.Deployment
	if e := h.c.Get(h.ctx, key, &dep); e != nil {
		h.t.Fatal(e)
	}
	dep.Spec.Replicas = &replicas
	if e := h.c.Update(h.ctx, &dep); e != nil {
		h.t.Fatal(e)
	}
	h.wait(fmt.Sprintf("Operator replicas=%d", replicas), func() error {
		if e := h.c.Get(h.ctx, key, &dep); e != nil {
			return e
		}
		if replicas == 0 {
			var pods corev1.PodList
			if e := h.c.List(h.ctx, &pods, client.InNamespace(domain.Namespace), client.MatchingLabels{"app.kubernetes.io/name": "hugofleet-operator"}); e != nil {
				return e
			}
			if len(pods.Items) != 0 {
				return fmt.Errorf("Operator Pods still exist")
			}
			return nil
		}
		if dep.Status.ObservedGeneration < dep.Generation || dep.Status.ReadyReplicas != 1 || dep.Status.AvailableReplicas != 1 {
			return fmt.Errorf("Operator not ready")
		}
		return nil
	})
}
func inputs(raw []byte, action string) (url.Values, error) {
	doc, e := html.Parse(bytes.NewReader(raw))
	if e != nil {
		return nil, e
	}
	v := url.Values{}
	found := false
	var walk func(*html.Node, bool)
	attr := func(n *html.Node, key string) string {
		for _, a := range n.Attr {
			if a.Key == key {
				return a.Val
			}
		}
		return ""
	}
	walk = func(n *html.Node, in bool) {
		if n.Type == html.ElementNode && n.Data == "form" {
			in = attr(n, "action") == action
			if in {
				found = true
			}
		}
		if in && n.Type == html.ElementNode && n.Data == "input" && attr(n, "type") != "checkbox" {
			name := attr(n, "name")
			if name != "" {
				v.Set(name, attr(n, "value"))
			}
		}
		if in && n.Type == html.ElementNode && n.Data == "select" {
			name := attr(n, "name")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Data == "option" {
					v.Set(name, attr(c, "value"))
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, in)
		}
	}
	walk(doc, false)
	if !found {
		return nil, fmt.Errorf("form %s missing", action)
	}
	return v, nil
}
func (h *harness) uiForm(page, action string, override url.Values) string {
	h.t.Helper()
	status, raw, e := h.request("GET", page, "", nil)
	if e != nil || status != 200 {
		h.t.Fatalf("UI read %d %s %v", status, raw, e)
	}
	if e = os.WriteFile(filepath.Join(h.artifacts, "ui-before.html"), raw, 0600); e != nil {
		h.t.Fatal(e)
	}
	form, e := inputs(raw, action)
	if e != nil {
		h.t.Fatal(e)
	}
	for k, v := range override {
		form[k] = v
	}
	r, e := http.NewRequestWithContext(h.ctx, "POST", base+action, strings.NewReader(form.Encode()))
	if e != nil {
		h.t.Fatal(e)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", base)
	res, e := h.http.Do(r)
	if e != nil {
		h.t.Fatal(e)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 303 {
		h.t.Fatalf("UI submit %d %s", res.StatusCode, body)
	}
	location := res.Header.Get("Location")
	if !strings.HasPrefix(location, "/apps/") {
		h.t.Fatalf("invalid redirect %s", location)
	}
	return strings.TrimPrefix(location, "/apps/")
}

func TestM2HubKind(t *testing.T) {
	if os.Getenv("HF_CLUSTER") != "hugofleet-m2-e2e" {
		t.Fatal("only script-owned hugofleet-m2-e2e is allowed")
	}
	root, artifacts := os.Getenv("HF_ROOT"), os.Getenv("HF_ARTIFACTS")
	if root == "" || artifacts == "" {
		t.Fatal("script environment missing")
	}
	raw, e := clientcmd.LoadFromFile(os.Getenv("KUBECONFIG"))
	if e != nil || raw.CurrentContext != "kind-hugofleet-m2-e2e" {
		t.Fatalf("unexpected bootstrap context: %v", e)
	}
	cfg, e := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if e != nil {
		t.Fatal(e)
	}
	cfg.Timeout = 10 * time.Second
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{platform.AddToScheme, appsv1.AddToScheme, corev1.AddToScheme} {
		if e = add(scheme); e != nil {
			t.Fatal(e)
		}
	}
	c, e := client.New(cfg, client.Options{Scheme: scheme})
	if e != nil {
		t.Fatal(e)
	}
	admin, e := kubernetes.NewForConfig(cfg)
	if e != nil {
		t.Fatal(e)
	}
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{Timeout: 10 * time.Second, Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(context.Background(), 22*time.Minute)
	defer cancel()
	h := &harness{t: t, ctx: ctx, root: root, artifacts: artifacts, http: httpClient, c: c, admin: admin}
	defer h.stop(false)
	// These real authorization checks use the Hub's short-lived SA credentials.
	limited, e := clientcmd.BuildConfigFromFlags("", os.Getenv("HF_HUB_KUBECONFIG"))
	if e != nil {
		t.Fatal(e)
	}
	limited.Timeout = 10 * time.Second
	sa, e := kubernetes.NewForConfig(limited)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		verb, group, resource, sub, ns string
		allowed                        bool
	}{{"create", "platform.hugofleet.io", "applications", "", domain.Namespace, true}, {"get", "apps", "deployments", "", domain.Namespace, true}, {"get", "", "secrets", "", domain.Namespace, false}, {"create", "", "pods", "exec", domain.Namespace, false}, {"update", "apps", "deployments", "", domain.Namespace, false}, {"create", "", "namespaces", "", "", false}, {"create", "platform.hugofleet.io", "applications", "", "default", false}} {
		v, e := sa.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authzv1.SelfSubjectAccessReview{Spec: authzv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authzv1.ResourceAttributes{Verb: tc.verb, Group: tc.group, Resource: tc.resource, Subresource: tc.sub, Namespace: tc.ns}}}, metav1.CreateOptions{})
		if e != nil || v.Status.Allowed != tc.allowed {
			t.Fatalf("Hub RBAC %+v: %+v %v", tc, v, e)
		}
	}
	t.Log("PASS actual limited Hub RBAC")
	h.start("180s")
	// Reject drive-by JSON/form requests and DNS rebinding before any mutation.
	for _, tc := range []struct{ host, origin, site string }{{"evil.example", "", ""}, {"", "https://evil.example", ""}, {"", "", "cross-site"}} {
		r, _ := http.NewRequestWithContext(ctx, "POST", base+"/api/v1/tenants/demo/apps", strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		if tc.host != "" {
			r.Host = tc.host
		}
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		res, e := httpClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatalf("cross-origin/host request accepted %d", res.StatusCode)
		}
	}
	res, e := httpClient.PostForm(base+"/ui/apps", url.Values{"csrf": {"forged"}})
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("forged CSRF accepted")
	}
	appID := h.uiForm("/", "/ui/apps", nil)
	detail, e := h.detail(appID)
	if e != nil || len(detail.Operations) != 1 {
		t.Fatalf("UI did not durably deploy %+v %v", detail, e)
	}
	one := domain.Accepted{ApplicationID: appID, OperationID: detail.Operations[0].ID, Revision: 1}
	h.terminal(one, "SUCCEEDED")
	// Verify actual sample HTTP traffic through the Kubernetes Service.
	log, e := os.Create(filepath.Join(artifacts, "forward.log"))
	if e != nil {
		t.Fatal(e)
	}
	defer log.Close()
	var forward *exec.Cmd
	stopForward := func() {
		if forward != nil && forward.Process != nil {
			_ = forward.Process.Kill()
			_ = forward.Wait()
		}
		forward = nil
	}
	defer stopForward()
	startForward := func() {
		stopForward()
		forward = exec.CommandContext(ctx, "kubectl", "--kubeconfig", os.Getenv("KUBECONFIG"), "--context", "kind-hugofleet-m2-e2e", "-n", domain.Namespace, "port-forward", "--address", "127.0.0.1", "service/pricing-api", "18180:8080")
		forward.Stdout = log
		forward.Stderr = log
		if e = forward.Start(); e != nil {
			t.Fatal(e)
		}
	}
	startForward()
	quote := func(version string) error {
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:18180/version", nil)
		res, e := httpClient.Do(req)
		if e != nil {
			return e
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || !strings.Contains(string(body), `"version":"`+version+`"`) {
			return fmt.Errorf("version %s", body)
		}
		req, _ = http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:18180/quote", strings.NewReader(`{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}`))
		req.Header.Set("Content-Type", "application/json")
		res, e = httpClient.Do(req)
		if e != nil {
			return e
		}
		defer res.Body.Close()
		body, _ = io.ReadAll(res.Body)
		var quote struct {
			Original int64 `json:"original_amount_cents"`
			Tier     int64 `json:"tier_discount_cents"`
			Coupon   int64 `json:"coupon_discount_cents"`
			Final    int64 `json:"final_amount_cents"`
		}
		if res.StatusCode != 200 || json.Unmarshal(body, &quote) != nil || quote.Original != 20000 || quote.Tier != 2000 || quote.Coupon != 10000 || quote.Final != 8000 {
			return fmt.Errorf("quote %d %s", res.StatusCode, body)
		}
		return nil
	}
	h.wait("UI v1 deployment serves real /quote=8000", func() error { return quote("v1") })
	h.observeSpecFault(appID)
	path := "/api/v1/tenants/demo/apps/" + appID
	expected := int64(1)
	request := domain.DeployRequest{ExpectedDesiredRevision: &expected, Image: "pricing-api:v2-broken"}
	two := h.accepted(path+"/releases", "broken", request)
	again := h.accepted(path+"/releases", "broken", request)
	if again.OperationID != two.OperationID {
		t.Fatal("API dedup changed operation")
	}
	different := request
	different.Image = "pricing-api:v2-good"
	status, _, e := h.request("POST", path+"/releases", "broken", different)
	if e != nil || status != 409 {
		t.Fatalf("same key different payload %d %v", status, e)
	}
	failed := h.terminal(two, "FAILED")
	if failed.ErrorCode != "ProgressDeadlineExceeded" {
		t.Fatalf("broken failure not diagnosed %+v", failed)
	}
	detail, e = h.detail(appID)
	if e != nil || len(detail.Releases) != 2 || detail.Application.DesiredRevision != 2 || detail.Application.AppliedRevision != 2 || detail.Application.LastSuccessfulRevision != 1 {
		t.Fatalf("bad failed revision %+v %v", detail, e)
	}
	status, ui, e := h.request("GET", "/apps/"+appID, "", nil)
	if e != nil || status != 200 || !bytes.Contains(ui, []byte("FAILED")) || !bytes.Contains(ui, []byte("ProgressDeadlineExceeded")) {
		t.Fatal("UI omitted real failed rollout")
	}
	os.WriteFile(filepath.Join(artifacts, "ui-failed.html"), ui, 0600)
	h.wait("broken revision keeps previous /quote available", func() error { return quote("v1") })
	h.uiForm("/apps/"+appID, "/ui/apps/"+appID+"/rollback", url.Values{"target_revision": {"1"}, "confirm": {"yes"}})
	detail, e = h.detail(appID)
	if e != nil || detail.Releases[0].Revision != 3 || detail.Releases[0].RollbackSourceRevision != 1 {
		t.Fatalf("UI rollback did not create revision 3 %+v %v", detail, e)
	}
	three := domain.Accepted{ApplicationID: appID, OperationID: detail.Operations[0].ID, Revision: 3}
	h.terminal(three, "SUCCEEDED")
	// A Service port-forward still binds one Pod. The new revision replaces the
	// old v1 Pod, so reconnect before checking the unchanged business assertions.
	startForward()
	h.wait("rollback revision 3 serves real /quote", func() error { return quote("v1") })
	// Stop the real Operator to hold rollout pending while testing conflict and
	// Hub crash recovery. No mock status or substitute Kubernetes API is used.
	h.operator(0)
	expected = 3
	request = domain.DeployRequest{ExpectedDesiredRevision: &expected, Image: "pricing-api:v2-good"}
	type response struct {
		status int
		raw    []byte
		err    error
	}
	results := make(chan response, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, key := range []string{"concurrent-a", "concurrent-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			<-start
			s, b, e := h.request("POST", path+"/releases", key, request)
			results <- response{s, b, e}
		}(key)
	}
	close(start)
	wg.Wait()
	close(results)
	var four domain.Accepted
	acceptedCount, conflictCount := 0, 0
	for r := range results {
		if r.err != nil {
			t.Fatal(r.err)
		}
		switch r.status {
		case 202:
			acceptedCount++
			if e = json.Unmarshal(r.raw, &four); e != nil {
				t.Fatal(e)
			}
		case 409:
			conflictCount++
		default:
			t.Fatalf("concurrent response %d %s", r.status, r.raw)
		}
	}
	if acceptedCount != 1 || conflictCount != 1 || four.Revision != 4 {
		t.Fatalf("concurrency did not select one update %+v %d/%d", four, acceptedCount, conflictCount)
	}
	h.wait("revision 4 applied and RUNNING before Hub crash", func() error {
		d, e := h.detail(appID)
		if e != nil {
			return e
		}
		op, e := h.operation(four.OperationID)
		if e != nil {
			return e
		}
		if d.Application.AppliedRevision != 4 || op.State != "RUNNING" {
			return fmt.Errorf("pending applied=%d state=%s", d.Application.AppliedRevision, op.State)
		}
		return nil
	})
	before, e := h.operation(four.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	var cr platform.Application
	if e = c.Get(ctx, client.ObjectKey{Namespace: domain.Namespace, Name: "pricing-api"}, &cr); e != nil {
		t.Fatal(e)
	}
	uid, generation := cr.UID, cr.Generation
	h.stop(true)
	h.start("180s")
	after, e := h.operation(four.OperationID)
	if e != nil || before.CommandID != after.CommandID || before.DeadlineAt != after.DeadlineAt {
		t.Fatalf("recovery changed identity/deadline %+v %+v %v", before, after, e)
	}
	h.operator(1)
	h.terminal(four, "SUCCEEDED")
	if e = c.Get(ctx, client.ObjectKey{Namespace: domain.Namespace, Name: "pricing-api"}, &cr); e != nil || cr.UID != uid || cr.Generation != generation {
		t.Fatalf("Hub restart rewrote CR %v", e)
	}
	detail, e = h.detail(appID)
	if e != nil || len(detail.Releases) != 4 || len(detail.Operations) != 4 {
		t.Fatalf("Hub restart duplicated history %+v %v", detail, e)
	}
	// A real ResourceQuota rejects the additional surge Pod, even though the
	// per-app Hub precheck passes. Observe FailedCreate instead of false success.
	quota := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "m2-surge-test", Namespace: domain.Namespace}, Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{corev1.ResourceRequestsCPU: resource.MustParse("200m")}}}
	if e = c.Create(ctx, quota); e != nil {
		t.Fatal(e)
	}
	h.wait("quota accounts for existing Pods", func() error {
		if e = c.Get(ctx, client.ObjectKeyFromObject(quota), quota); e != nil {
			return e
		}
		used := quota.Status.Used[corev1.ResourceRequestsCPU]
		if used.Cmp(resource.MustParse("200m")) != 0 {
			return fmt.Errorf("quota used=%s", used.String())
		}
		return nil
	})
	expected = 4
	request = domain.DeployRequest{ExpectedDesiredRevision: &expected, Image: "pricing-api:v1"}
	five := h.accepted(path+"/releases", "quota", request)
	quotaFailure := h.terminal(five, "FAILED")
	if quotaFailure.ErrorCode != "FailedCreate" || !strings.Contains(quotaFailure.ErrorMessage, "quota") {
		t.Fatalf("quota failure not observable %+v", quotaFailure)
	}
	if e = c.Delete(ctx, quota); e != nil {
		t.Fatal(e)
	}
	// Deleting the quota does not synchronously clear FailedCreate. First observe
	// the failed revision's real recovery, then submit a new revision with its own
	// Pod template to confirm it without rewriting the failed history.
	h.wait("quota removed and failed revision becomes Ready with history preserved", func() error {
		if e := c.Get(ctx, client.ObjectKeyFromObject(quota), &corev1.ResourceQuota{}); !apierrors.IsNotFound(e) {
			return fmt.Errorf("quota still present or unavailable: %v", e)
		}
		d, e := h.detail(appID)
		if e != nil {
			return e
		}
		if d.Application.Status != "READY" || d.Application.DesiredRevision != 5 || d.Application.AppliedRevision != 5 || d.Application.ObservationErrorCode != "" {
			return fmt.Errorf("waiting for actual quota recovery: %+v", d.Application)
		}
		op, e := h.operation(five.OperationID)
		if e != nil {
			return e
		}
		if d.Application.LastSuccessfulRevision != 4 || len(d.Releases) != 5 || len(d.Operations) != 5 || d.Releases[0].State != "FAILED" || op.State != "FAILED" || op.ErrorCode != "FailedCreate" || op.CommandID != quotaFailure.CommandID || op.DeadlineAt != quotaFailure.DeadlineAt {
			t.Fatalf("quota recovery rewrote terminal history: %+v %+v", d, op)
		}
		return nil
	})
	expected = 5
	request.ExpectedDesiredRevision = &expected
	six := h.accepted(path+"/releases", "quota-restored", request)
	h.terminal(six, "SUCCEEDED")
	// Deadline is terminal, even if actual Kubernetes rollout later succeeds.
	h.operator(0)
	h.stop(false)
	h.start("20s")
	expected = 6
	request = domain.DeployRequest{ExpectedDesiredRevision: &expected, Image: "pricing-api:v2-good"}
	seven := h.accepted(path+"/releases", "late-ready", request)
	h.wait("revision 7 CR applied before timeout", func() error {
		d, e := h.detail(appID)
		if e != nil {
			return e
		}
		op, e := h.operation(seven.OperationID)
		if e != nil {
			return e
		}
		if d.Application.AppliedRevision != 7 || op.State != "RUNNING" {
			return fmt.Errorf("applied=%d state=%s", d.Application.AppliedRevision, op.State)
		}
		return nil
	})
	h.terminal(seven, "TIMED_OUT")
	h.operator(1)
	h.wait("late Ready is an observation, not rewritten history", func() error {
		d, e := h.detail(appID)
		if e != nil {
			return e
		}
		if d.Application.Status != "READY" {
			return fmt.Errorf("status=%s", d.Application.Status)
		}
		if d.Application.LastSuccessfulRevision != 6 || d.Releases[0].State != "FAILED" {
			t.Fatalf("late completion changed history %+v", d)
		}
		op, e := h.operation(seven.OperationID)
		if e != nil {
			return e
		}
		if op.State != "TIMED_OUT" {
			t.Fatalf("terminal timeout mutated %+v", op)
		}
		for _, ev := range d.Events {
			if ev.Kind == "LATE_READY" && ev.OperationID == seven.OperationID {
				return nil
			}
		}
		return fmt.Errorf("late-ready event missing")
	})
	h.recoverDeletionUID()
	h.recoverObservationUID()
	expected = 7
	request.ExpectedDesiredRevision = &expected
	eight := h.accepted(path+"/releases", "confirm-late-version", request)
	h.terminal(eight, "SUCCEEDED")
	detail, e = h.detail(appID)
	if e != nil || detail.Application.LastSuccessfulRevision != 8 {
		t.Fatalf("new operation did not confirm usable revision %+v %v", detail, e)
	}
	h.uiForm("/apps/"+appID, "/ui/apps/"+appID+"/delete", url.Values{"confirm": {"yes"}})
	detail, e = h.detail(appID)
	if e != nil {
		t.Fatal(e)
	}
	deleted := domain.Accepted{ApplicationID: appID, OperationID: detail.Operations[0].ID, Revision: 8}
	h.terminal(deleted, "SUCCEEDED")
	detail, e = h.detail(appID)
	if e != nil || detail.Application.Status != "DELETED" || len(detail.Releases) != 8 {
		t.Fatalf("delete lost history %+v %v", detail, e)
	}
	h.stop(false)
	h.start("180s")
	detail, e = h.detail(appID)
	if e != nil || detail.Application.Status != "DELETED" || len(detail.Releases) != 8 || len(detail.Operations) != 9 {
		t.Fatalf("completed history not durable %+v %v", detail, e)
	}
	status, ui, e = h.request("GET", "/apps/"+appID, "", nil)
	if e != nil || status != 200 {
		t.Fatalf("final UI %d %v", status, e)
	}
	os.WriteFile(filepath.Join(artifacts, "ui-final.html"), ui, 0600)
	t.Log("PASS M2 real UI /quote, failure, new-revision rollback, idempotency, conflict, crash recovery, quota, terminal late Ready, deletion and durable history")
}

func (h *harness) recoverDeletionUID() {
	h.t.Helper()
	// Inject a real SQLite write failure in the create acknowledgement window.
	// Kubernetes still runs the actual CR and workload; no status is fabricated.
	// DDL runs with the Hub stopped, respecting the single-writer design.
	h.stop(false)
	u := url.URL{Scheme: "file", Path: filepath.Join(h.artifacts, "hub.db")}
	u.RawQuery = "_pragma=busy_timeout%285000%29"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		h.t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(h.ctx, "CREATE TRIGGER m2_test_uid_interruption BEFORE UPDATE OF kubernetes_uid ON applications WHEN OLD.name='uid-recovery' AND OLD.kubernetes_uid='' AND NEW.kubernetes_uid!='' BEGIN SELECT RAISE(ABORT,'M2 test create UID acknowledgement interrupted'); END")
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		db.ExecContext(cleanup, "DROP TRIGGER IF EXISTS m2_test_uid_interruption")
	}()
	h.start("20s")
	expected := int64(0)
	created := h.accepted("/api/v1/tenants/demo/apps", "uid-recovery-create", domain.DeployRequest{Name: "uid-recovery", ClusterID: domain.ClusterID, ExpectedDesiredRevision: &expected, Image: "pricing-api:v1"})
	var cr platform.Application
	h.wait("actual workload exists despite interrupted Hub UID acknowledgement", func() error {
		key := client.ObjectKey{Namespace: domain.Namespace, Name: "uid-recovery"}
		if err := h.c.Get(h.ctx, key, &cr); err != nil {
			return err
		}
		var dep appsv1.Deployment
		if err := h.c.Get(h.ctx, key, &dep); err != nil {
			return err
		}
		owner := metav1.GetControllerOf(&dep)
		if owner == nil || owner.UID != cr.UID {
			return fmt.Errorf("workload ownership is not established")
		}
		return nil
	})
	h.terminal(created, "TIMED_OUT")
	d, err := h.detail(created.ApplicationID)
	if err != nil || d.Application.KubernetesUID != "" || d.Application.AppliedRevision != 0 {
		h.t.Fatalf("write interruption did not hold UID acknowledgement: %+v %v", d, err)
	}
	// Restore the normal timeout before accepting deletion, but keep the
	// acknowledgement fault active until deletion has been durably accepted.
	h.stop(false)
	h.start("180s")
	expected = 1
	status, raw, err := h.request("DELETE", "/api/v1/tenants/demo/apps/"+created.ApplicationID, "uid-recovery-delete", domain.DeleteRequest{ExpectedDesiredRevision: &expected})
	if err != nil || status != 202 {
		h.t.Fatalf("recovery deletion: %d %s %v", status, raw, err)
	}
	var deleted domain.Accepted
	if err = json.Unmarshal(raw, &deleted); err != nil {
		h.t.Fatal(err)
	}
	// Accept deletion while acknowledgement is still blocked, so recovery must
	// bind the deletion identity rather than acknowledge a deployment first.
	h.stop(false)
	if _, err = db.ExecContext(h.ctx, "DROP TRIGGER m2_test_uid_interruption"); err != nil {
		h.t.Fatal(err)
	}
	h.start("180s")
	h.terminal(deleted, "SUCCEEDED")
	d, err = h.detail(created.ApplicationID)
	if err != nil || d.Application.Status != "DELETED" || d.Application.KubernetesUID != string(cr.UID) || d.Application.AppliedRevision != 0 || d.Application.LastSuccessfulRevision != 0 {
		h.t.Fatalf("deletion did not preserve recovered UID/history: %+v %v", d, err)
	}
	for _, ev := range d.Events {
		if ev.Kind == "UID_BOUND" && ev.OperationID == deleted.OperationID {
			h.t.Log("PASS real SQLite create acknowledgement failure recovers CR UID before actual deletion GC")
			return
		}
	}
	h.t.Fatal("deletion UID binding event missing")
}

func (h *harness) observeSpecFault(appID string) {
	h.t.Helper()
	h.operator(0)
	key := client.ObjectKey{Namespace: domain.Namespace, Name: "pricing-api"}
	var cr platform.Application
	if err := h.c.Get(h.ctx, key, &cr); err != nil {
		h.t.Fatal(err)
	}
	original := cr.Spec
	cr.Spec.Image = "pricing-api:v2-good"
	if err := h.c.Update(h.ctx, &cr); err != nil {
		h.t.Fatal(err)
	}
	h.wait("current spec drift reason is persisted without changing success history", func() error {
		d, err := h.detail(appID)
		if err != nil {
			return err
		}
		if d.Application.ObservationErrorCode != "SPEC_DRIFT" || d.Application.Status != "DEGRADED" {
			return fmt.Errorf("observation=%+v", d.Application)
		}
		if d.Operations[0].State != "SUCCEEDED" || d.Releases[0].State != "ACTIVE" || d.Application.LastSuccessfulRevision != 1 {
			h.t.Fatalf("observation rewrote history: %+v", d)
		}
		return nil
	})
	h.stop(false)
	h.start("180s")
	status, body, err := h.request("GET", "/apps/"+appID, "", nil)
	if err != nil || status != 200 || !strings.Contains(string(body), "SPEC_DRIFT") {
		h.t.Fatalf("UI lost current observation reason: %d %s %v", status, body, err)
	}
	if err = h.c.Get(h.ctx, key, &cr); err != nil {
		h.t.Fatal(err)
	}
	cr.Spec = original
	if err = h.c.Update(h.ctx, &cr); err != nil {
		h.t.Fatal(err)
	}
	h.operator(1)
	h.wait("restored spec clears current failure and records recovery", func() error {
		d, err := h.detail(appID)
		if err != nil {
			return err
		}
		if d.Application.Status != "READY" || d.Application.ObservationErrorCode != "" {
			return fmt.Errorf("observation=%+v", d.Application)
		}
		faults, recovered := 0, 0
		for _, event := range d.Events {
			if event.Kind == "OBSERVED_DEGRADED" {
				faults++
			}
			if event.Kind == "OBSERVED_RECOVERED" {
				recovered++
			}
		}
		if faults != 1 || recovered != 1 {
			return fmt.Errorf("observation transitions=%d/%d", faults, recovered)
		}
		return nil
	})
	h.t.Log("PASS real CR drift, durable current reason, UI and observation recovery")
}

func (h *harness) recoverObservationUID() {
	h.t.Helper()
	h.stop(false)
	u := url.URL{Scheme: "file", Path: filepath.Join(h.artifacts, "hub.db")}
	u.RawQuery = "_pragma=busy_timeout%285000%29"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		h.t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(h.ctx, "CREATE TRIGGER m2_test_observation_interruption BEFORE UPDATE OF kubernetes_uid ON applications WHEN OLD.name='observed-recovery' AND OLD.kubernetes_uid='' AND NEW.kubernetes_uid!='' BEGIN SELECT RAISE(ABORT,'M2 test observation acknowledgement interrupted'); END"); err != nil {
		h.t.Fatal(err)
	}
	h.start("20s")
	expected := int64(0)
	created := h.accepted("/api/v1/tenants/demo/apps", "observation-recovery-create", domain.DeployRequest{Name: "observed-recovery", ClusterID: domain.ClusterID, ExpectedDesiredRevision: &expected, Image: "pricing-api:v1"})
	var cr platform.Application
	h.wait("real CR exists before acknowledgement and Hub crash", func() error {
		if err := h.c.Get(h.ctx, client.ObjectKey{Namespace: domain.Namespace, Name: "observed-recovery"}, &cr); err != nil {
			return err
		}
		d, err := h.detail(created.ApplicationID)
		if err != nil {
			return err
		}
		if d.Application.KubernetesUID != "" || d.Application.AppliedRevision != 0 {
			h.t.Fatalf("acknowledgement was not interrupted: %+v", d)
		}
		return nil
	})
	before, err := h.operation(created.OperationID)
	if err != nil {
		h.t.Fatal(err)
	}
	if !before.Active() || time.Now().UnixMilli() >= before.DeadlineAt {
		h.t.Fatalf("lost acknowledgement must precede the original deadline: %+v", before)
	}
	uid, generation := cr.UID, cr.Generation
	h.stop(true)
	// The restart must occur after the original persisted deadline, not after
	// accepting a fresh operation or fabricating any Kubernetes status.
	if delay := time.Until(time.UnixMilli(before.DeadlineAt)); delay > 0 {
		select {
		case <-time.After(delay + time.Millisecond):
		case <-h.ctx.Done():
			h.t.Fatal(h.ctx.Err())
		}
	}
	if _, err = db.ExecContext(h.ctx, "DROP TRIGGER m2_test_observation_interruption"); err != nil {
		h.t.Fatal(err)
	}
	h.start("180s")
	h.wait("late read-only observation recovers exact CR after expired lost acknowledgement", func() error {
		d, err := h.detail(created.ApplicationID)
		if err != nil {
			return err
		}
		if d.Application.Status != "READY" || d.Application.AppliedRevision != 1 || d.Application.KubernetesUID != string(uid) {
			return fmt.Errorf("observation=%+v", d.Application)
		}
		op := d.Operations[0]
		if op.State != "TIMED_OUT" || op.CommandID != before.CommandID || op.DeadlineAt != before.DeadlineAt || d.Releases[0].State != "FAILED" || d.Application.LastSuccessfulRevision != 0 {
			h.t.Fatalf("recovery changed terminal history: %+v", d)
		}
		late := 0
		for _, event := range d.Events {
			if event.Kind == "LATE_READY" {
				late++
			}
		}
		if late != 1 {
			return fmt.Errorf("late observation count=%d", late)
		}
		return nil
	})
	if err = h.c.Get(h.ctx, client.ObjectKey{Namespace: domain.Namespace, Name: cr.Name}, &cr); err != nil || cr.UID != uid || cr.Generation != generation {
		h.t.Fatalf("read-only recovery rewrote CR: %v %+v", err, cr.ObjectMeta)
	}
	expected = 1
	status, raw, err := h.request("DELETE", "/api/v1/tenants/demo/apps/"+created.ApplicationID, "observation-recovery-delete", domain.DeleteRequest{ExpectedDesiredRevision: &expected})
	if err != nil || status != 202 {
		h.t.Fatalf("observation recovery cleanup: %d %s %v", status, raw, err)
	}
	var deleted domain.Accepted
	if err = json.Unmarshal(raw, &deleted); err != nil {
		h.t.Fatal(err)
	}
	h.terminal(deleted, "SUCCEEDED")
	h.t.Log("PASS real lost acknowledgement, SIGKILL, expired deadline, exact UID recovery and immutable terminal history")
}
