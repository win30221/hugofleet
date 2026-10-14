package controller

import (
	"testing"
	"time"

	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testApp() *platform.Application {
	return &platform.Application{ObjectMeta: metav1.ObjectMeta{Name: "pricing-api", Namespace: "hf-demo", UID: "test-uid", Generation: 2},
		Spec: platform.ApplicationSpec{Revision: 2, Image: "pricing-api:v2-good", Replicas: 2, Port: 8080, ReadinessPath: "/readyz", LivenessPath: "/healthz",
			Resources: platform.ApplicationResources{Requests: platform.ResourceValues{CPU: "100m", Memory: "64Mi"}, Limits: platform.ResourceValues{CPU: "300m", Memory: "128Mi"}}},
		Status: platform.ApplicationStatus{ObservedRevision: 1, Image: "pricing-api:v1"}}
}

func readyDeployment(app *platform.Application) *appsv1.Deployment {
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Namespace, UID: "test-deployment-uid", Generation: 3}}
	mutateDeployment(app, d)
	d.Status = appsv1.DeploymentStatus{ObservedGeneration: 3, Replicas: 2, UpdatedReplicas: 2, ReadyReplicas: 2, AvailableReplicas: 2}
	return d
}

func TestRolloutTruth(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*appsv1.Deployment)
		ready    bool
		observed int64
		reason   string
	}{
		{"complete", func(d *appsv1.Deployment) {}, true, 2, "RolloutComplete"},
		{"old observed generation", func(d *appsv1.Deployment) { d.Status.ObservedGeneration = 2 }, false, 1, "Reconciling"},
		{"old image", func(d *appsv1.Deployment) { d.Spec.Template.Spec.Containers[0].Image = "pricing-api:v1" }, false, 1, "Reconciling"},
		{"old revision", func(d *appsv1.Deployment) { d.Annotations[RevisionAnnotation] = "1" }, false, 1, "Reconciling"},
		{"wrong uid", func(d *appsv1.Deployment) { d.Labels[UIDLabel] = "other" }, false, 1, "Reconciling"},
		{"old replicas", func(d *appsv1.Deployment) { d.Spec.Replicas = ptr[int32](1) }, false, 1, "Reconciling"},
		{"old pods available", func(d *appsv1.Deployment) { d.Status.UpdatedReplicas = 0 }, false, 2, "RolloutInProgress"},
		{"extra retiring pod", func(d *appsv1.Deployment) { d.Status.Replicas = 3 }, false, 2, "RolloutInProgress"},
		{"not available", func(d *appsv1.Deployment) { d.Status.AvailableReplicas = 1 }, false, 2, "RolloutInProgress"},
		{"broken rollout", func(d *appsv1.Deployment) {
			d.Status.Replicas = 3
			d.Status.UpdatedReplicas = 1
			d.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded", Message: "broken readiness", LastUpdateTime: metav1.Now()}}
		}, false, 2, "ProgressDeadlineExceeded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := testApp()
			dep := readyDeployment(app)
			tc.mutate(dep)
			current := deadlineReplicaSet(dep, time.Now().Add(-2*time.Minute))
			s := rolloutStatus(app, dep, *current)
			c := meta.FindStatusCondition(s.Conditions, "Ready")
			if (c.Status == metav1.ConditionTrue) != tc.ready || c.Reason != tc.reason || s.ObservedRevision != tc.observed || s.ObservedGeneration != app.Generation {
				t.Fatalf("incorrect status: %+v", s)
			}
			for _, c := range s.Conditions {
				if c.ObservedGeneration != app.Generation || c.LastTransitionTime.IsZero() {
					t.Fatalf("incomplete condition: %+v", c)
				}
			}
		})
	}
}

func TestStableStatusAndWorkload(t *testing.T) {
	a := testApp()
	d := readyDeployment(a)
	a.Status = rolloutStatus(a, d)
	if s := rolloutStatus(a, d); !equality.Semantic.DeepEqual(s, a.Status) {
		t.Fatalf("no-op rollout changed status")
	}
	d.Labels["user"] = "preserved"
	d.Annotations["user"] = "preserved"
	d.Spec.Template.Annotations = map[string]string{"user": "preserved"}
	mutateDeployment(a, d)
	before := d.DeepCopy()
	mutateDeployment(a, d)
	if !equality.Semantic.DeepEqual(d, before) || d.Labels["user"] != "preserved" || d.Spec.Template.Annotations["user"] != "preserved" {
		t.Fatal("workload mutation not idempotent or metadata lost")
	}
	s := &corev1.Service{Spec: corev1.ServiceSpec{ClusterIP: "10.96.0.10", ClusterIPs: []string{"10.96.0.10"}, IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol}}}
	mutateService(a, s)
	sBefore := s.DeepCopy()
	mutateService(a, s)
	if s.Spec.ClusterIP != "10.96.0.10" || !equality.Semantic.DeepEqual(s, sBefore) {
		t.Fatal("service IP changed or mutation not idempotent")
	}
}
