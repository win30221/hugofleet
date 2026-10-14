//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var appKey = client.ObjectKey{Namespace: "hf-demo", Name: "pricing-api"}

func TestM1KindLifecycle(t *testing.T) {
	if os.Getenv("HF_CLUSTER") != "hugofleet-m1-e2e" {
		t.Fatal("only the dedicated, script-owned hugofleet-m1-e2e is permitted")
	}
	kubeconfig := os.Getenv("KUBECONFIG")
	loaded, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CurrentContext != "kind-hugofleet-m1-e2e" {
		t.Fatal("unexpected kubeconfig context")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Timeout = 15 * time.Second
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme, discoveryv1.AddToScheme, platform.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()
	wait := func(label string, check func() error) {
		t.Helper()
		attemptCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
		defer cancel()
		var last error
		for {
			last = check()
			if last == nil {
				t.Logf("PASS %s", label)
				return
			}
			select {
			case <-attemptCtx.Done():
				t.Fatalf("%s: %v (last: %v)", label, attemptCtx.Err(), last)
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	var app platform.Application
	var dep appsv1.Deployment
	var svc corev1.Service
	ready := func() error {
		if err := c.Get(ctx, appKey, &app); err != nil {
			return err
		}
		if err := c.Get(ctx, appKey, &dep); err != nil {
			return err
		}
		if err := c.Get(ctx, appKey, &svc); err != nil {
			return err
		}
		condition := meta.FindStatusCondition(app.Status.Conditions, "Ready")
		if condition == nil || condition.Status != metav1.ConditionTrue || condition.ObservedGeneration != app.Generation || app.Status.ObservedGeneration != app.Generation || app.Status.ObservedRevision != app.Spec.Revision || app.Status.Image != app.Spec.Image {
			return fmt.Errorf("CR status not current: %+v", app.Status)
		}
		for _, name := range []string{"Progressing", "Degraded"} {
			v := meta.FindStatusCondition(app.Status.Conditions, name)
			if v == nil || v.Status != metav1.ConditionFalse || v.ObservedGeneration != app.Generation {
				return fmt.Errorf("unexpected %s condition", name)
			}
		}
		n := app.Spec.Replicas
		if dep.Status.ObservedGeneration < dep.Generation || dep.Spec.Replicas == nil || *dep.Spec.Replicas != n || dep.Spec.Template.Spec.Containers[0].Image != app.Spec.Image || dep.Status.Replicas != n || dep.Status.UpdatedReplicas != n || dep.Status.ReadyReplicas != n || dep.Status.AvailableReplicas != n {
			return fmt.Errorf("Deployment rollout incomplete")
		}
		for _, obj := range []client.Object{&dep, &svc} {
			o := metav1.GetControllerOf(obj)
			if o == nil || o.UID != app.UID || o.Kind != "Application" || o.APIVersion != platform.GroupVersion.String() {
				return fmt.Errorf("missing UID ownership")
			}
			if obj.GetAnnotations()["platform.hugofleet.io/revision"] != strconv.FormatInt(app.Spec.Revision, 10) {
				return fmt.Errorf("incorrect revision annotation")
			}
		}
		if svc.Spec.Selector["platform.hugofleet.io/application-uid"] != string(app.UID) || len(svc.Spec.Selector) != 1 || svc.Spec.Ports[0].Port != app.Spec.Port {
			return fmt.Errorf("service not converged")
		}
		var pods corev1.PodList
		if err := c.List(ctx, &pods, client.InNamespace(app.Namespace), client.MatchingLabels{"platform.hugofleet.io/application-uid": string(app.UID)}); err != nil {
			return err
		}
		if len(pods.Items) != int(n) {
			return fmt.Errorf("waiting for retired Pod deletion")
		}
		for _, p := range pods.Items {
			if !p.DeletionTimestamp.IsZero() || len(p.Spec.Containers) != 1 || p.Spec.Containers[0].Image != app.Spec.Image {
				return fmt.Errorf("unexpected Pod")
			}
			found := false
			for _, condition := range p.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("Pod not actually ready")
			}
		}
		return nil
	}
	update := func(revision int64, image string, replicas int32) {
		t.Helper()
		if err := c.Get(ctx, appKey, &app); err != nil {
			t.Fatal(err)
		}
		app.Spec.Revision = revision
		app.Spec.Image = image
		app.Spec.Replicas = replicas
		if err := c.Update(ctx, &app); err != nil {
			t.Fatal(err)
		}
	}
	sets := func() map[types.UID]bool {
		t.Helper()
		var list appsv1.ReplicaSetList
		if err := c.List(ctx, &list, client.InNamespace("hf-demo"), client.MatchingLabels{"platform.hugofleet.io/application-uid": string(app.UID)}); err != nil {
			t.Fatal(err)
		}
		out := map[types.UID]bool{}
		for _, r := range list.Items {
			out[r.UID] = true
		}
		return out
	}
	assertSameSets := func(before map[types.UID]bool) {
		t.Helper()
		after := sets()
		if len(before) != len(after) {
			t.Fatal("same template unexpectedly created a ReplicaSet")
		}
		for uid := range before {
			if !after[uid] {
				t.Fatal("same template unexpectedly replaced a ReplicaSet")
			}
		}
	}
	assertNewRevisionSet := func(before map[types.UID]bool, revision int64) {
		t.Helper()
		var list appsv1.ReplicaSetList
		if err := c.List(ctx, &list, client.InNamespace("hf-demo"), client.MatchingLabels{"platform.hugofleet.io/application-uid": string(app.UID)}); err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, r := range list.Items {
			if r.Spec.Template.Annotations["platform.hugofleet.io/revision"] != strconv.FormatInt(revision, 10) {
				continue
			}
			found++
			owner := metav1.GetControllerOf(&r)
			if before[r.UID] || r.UID == "" || owner == nil || owner.UID != dep.UID || owner.Kind != "Deployment" || owner.APIVersion != "apps/v1" || !r.DeletionTimestamp.IsZero() {
				t.Fatal("new revision did not create its own live Deployment-owned ReplicaSet")
			}
			if r.Status.ObservedGeneration < r.Generation || r.Status.Replicas != app.Spec.Replicas || r.Status.ReadyReplicas != app.Spec.Replicas || r.Status.AvailableReplicas != app.Spec.Replicas {
				t.Fatal("new revision ReplicaSet is not actually fully ready")
			}
		}
		if found != 1 {
			t.Fatalf("expected exactly one current revision ReplicaSet, found %d", found)
		}
	}
	wait("Application v1 -> real Deployment/Service/Ready Pods", ready)
	ip := svc.Spec.ClusterIP
	checkHTTP(t, "service/pricing-api", "v1")
	beforeSets := sets()
	update(2, "pricing-api:v1", 1)
	wait("identical configuration with new revision", ready)
	assertNewRevisionSet(beforeSets, 2)
	beforeSets = sets()
	generation := dep.Generation
	update(2, "pricing-api:v1", 1)
	wait("same revision replay remains Ready", ready)
	assertSameSets(beforeSets)
	if dep.Generation != generation {
		t.Fatal("same revision replay changed Deployment generation")
	}
	update(3, "pricing-api:v1", 2)
	wait("replicas-only revision -> two ready Pods", ready)
	assertNewRevisionSet(beforeSets, 3)
	beforeSets = sets()
	generation = dep.Generation
	update(3, "pricing-api:v1", 2)
	wait("same scaled revision replay remains Ready", ready)
	assertSameSets(beforeSets)
	if dep.Generation != generation {
		t.Fatal("same scaled revision replay changed Deployment generation")
	}
	checkHTTP(t, "service/pricing-api", "v1")
	update(4, "pricing-api:v2-good", 2)
	wait("image update -> v2-good", ready)
	checkHTTP(t, "service/pricing-api", "v2-good")
	if err := c.Patch(ctx, &dep, client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"annotations":{"user":"preserved"}},"spec":{"replicas":1}}`))); err != nil {
		t.Fatal(err)
	}
	wait("Deployment replicas drift repaired", ready)
	if dep.Annotations["user"] != "preserved" {
		t.Fatal("user metadata lost")
	}
	if err := c.Patch(ctx, &svc, client.RawPatch(types.MergePatchType, []byte(`{"spec":{"selector":{"foreign":"true"},"ports":[{"name":"http","port":9090,"targetPort":9090}]}}`))); err != nil {
		t.Fatal(err)
	}
	wait("Service selector/port drift repaired", ready)
	if svc.Spec.ClusterIP != ip {
		t.Fatal("ClusterIP changed during repair")
	}
	oldDepUID := dep.UID
	if err := c.Delete(ctx, &dep, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		t.Fatal(err)
	}
	wait("deleted Deployment recreated", func() error {
		if err := ready(); err != nil {
			return err
		}
		if dep.UID == oldDepUID {
			return fmt.Errorf("old UID still present")
		}
		return nil
	})
	oldSvcUID := svc.UID
	if err := c.Delete(ctx, &svc); err != nil {
		t.Fatal(err)
	}
	wait("deleted Service recreated", func() error {
		if err := ready(); err != nil {
			return err
		}
		if svc.UID == oldSvcUID {
			return fmt.Errorf("old UID still present")
		}
		return nil
	})
	ip = svc.Spec.ClusterIP
	var operators corev1.PodList
	if err := c.List(ctx, &operators, client.InNamespace("hf-demo"), client.MatchingLabels{"app.kubernetes.io/name": "hugofleet-operator"}); err != nil {
		t.Fatal(err)
	}
	if len(operators.Items) != 1 {
		t.Fatal("expected one Operator")
	}
	oldOperatorUID := operators.Items[0].UID
	if err := c.Delete(ctx, &operators.Items[0]); err != nil {
		t.Fatal(err)
	}
	if err := c.Patch(ctx, &dep, client.RawPatch(types.MergePatchType, []byte(`{"spec":{"replicas":1}}`))); err != nil {
		t.Fatal(err)
	}
	wait("Operator process restart and reconciliation", func() error {
		if err := c.List(ctx, &operators, client.InNamespace("hf-demo"), client.MatchingLabels{"app.kubernetes.io/name": "hugofleet-operator"}); err != nil {
			return err
		}
		if len(operators.Items) != 1 || operators.Items[0].UID == oldOperatorUID {
			return fmt.Errorf("waiting for replacement Operator")
		}
		return ready()
	})
	update(5, "pricing-api:v2-broken", 2)
	var broken corev1.Pod
	wait("broken Pod Running without readiness", func() error {
		var pods corev1.PodList
		if err := c.List(ctx, &pods, client.InNamespace("hf-demo"), client.MatchingLabels{"platform.hugofleet.io/application-uid": string(app.UID)}); err != nil {
			return err
		}
		for _, p := range pods.Items {
			if p.Spec.Containers[0].Image == "pricing-api:v2-broken" && p.Status.Phase == corev1.PodRunning {
				broken = p
				return nil
			}
		}
		return fmt.Errorf("broken Pod not running")
	})
	checkHTTP(t, "pod/"+broken.Name, "v2-broken")
	wait("CR Degraded reports actual ProgressDeadlineExceeded", func() error {
		if err := c.Get(ctx, appKey, &app); err != nil {
			return err
		}
		if err := c.Get(ctx, appKey, &dep); err != nil {
			return err
		}
		r := meta.FindStatusCondition(app.Status.Conditions, "Ready")
		d := meta.FindStatusCondition(app.Status.Conditions, "Degraded")
		if r == nil || d == nil || r.Status != metav1.ConditionFalse || d.Status != metav1.ConditionTrue || d.Reason != "ProgressDeadlineExceeded" || d.ObservedGeneration != app.Generation || app.Status.ObservedRevision != 5 || dep.Status.ObservedGeneration < dep.Generation {
			return fmt.Errorf("failure not observed: %+v", app.Status)
		}
		return nil
	})
	if err := c.Get(ctx, client.ObjectKeyFromObject(&broken), &broken); err != nil {
		t.Fatal(err)
	}
	for _, s := range broken.Status.ContainerStatuses {
		if s.Ready || s.RestartCount != 0 {
			t.Fatal("broken readiness must not become ready or restart")
		}
	}
	var endpoints discoveryv1.EndpointSliceList
	if err := c.List(ctx, &endpoints, client.InNamespace("hf-demo"), client.MatchingLabels{"kubernetes.io/service-name": "pricing-api"}); err != nil {
		t.Fatal(err)
	}
	readyEndpoints := 0
	for _, s := range endpoints.Items {
		for _, e := range s.Endpoints {
			if e.Conditions.Ready != nil && *e.Conditions.Ready {
				readyEndpoints++
				if e.TargetRef != nil && e.TargetRef.UID == broken.UID {
					t.Fatal("broken Pod became a ready endpoint")
				}
			}
		}
	}
	if readyEndpoints < 1 {
		t.Fatal("last good version has no ready endpoints")
	}
	checkHTTP(t, "service/pricing-api", "v2-good")
	update(6, "pricing-api:v1", 2)
	wait("new revision reuses old v1 configuration", ready)
	checkHTTP(t, "service/pricing-api", "v1")
	if svc.Spec.ClusterIP != ip {
		t.Fatal("image changes changed service IP")
	}
	if err := c.Delete(ctx, &app); err != nil {
		t.Fatal(err)
	}
	wait("Application deletion and Kubernetes GC", func() error {
		for _, o := range []client.Object{&platform.Application{}, &appsv1.Deployment{}, &corev1.Service{}} {
			if err := c.Get(ctx, appKey, o); !apierrors.IsNotFound(err) {
				return fmt.Errorf("object remains after delete: %T err=%v", o, err)
			}
		}
		var pods corev1.PodList
		var rs appsv1.ReplicaSetList
		if err := c.List(ctx, &pods, client.InNamespace("hf-demo"), client.MatchingLabels{"platform.hugofleet.io/application-uid": string(app.UID)}); err != nil {
			return err
		}
		if err := c.List(ctx, &rs, client.InNamespace("hf-demo"), client.MatchingLabels{"platform.hugofleet.io/application-uid": string(app.UID)}); err != nil {
			return err
		}
		if len(pods.Items) > 0 || len(rs.Items) > 0 {
			return fmt.Errorf("descendants remain")
		}
		return nil
	})
}

func checkHTTP(t *testing.T, target, version string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	log, err := os.CreateTemp(os.Getenv("HF_ARTIFACTS"), "http-forward-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", os.Getenv("KUBECONFIG"), "--context", "kind-hugofleet-m1-e2e", "-n", "hf-demo", "port-forward", "--address", "127.0.0.1", target, ":8080")
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	pattern := regexp.MustCompile(`Forwarding from 127\.0\.0\.1:([0-9]+)`)
	var port string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(log.Name())
		if err != nil {
			t.Fatal(err)
		}
		match := pattern.FindSubmatch(data)
		if len(match) == 2 {
			port = string(match[1])
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if port == "" {
		t.Fatalf("port-forward not ready; see %s", log.Name())
	}
	checker := exec.CommandContext(ctx, filepath.Join(os.Getenv("HF_ROOT"), "bin", "m0check"), "http", "http://127.0.0.1:"+port, version)
	output, err := checker.CombinedOutput()
	if err != nil {
		t.Fatalf("actual HTTP %s: %v\n%s", version, err, output)
	}
	t.Logf("%s", output)
}
