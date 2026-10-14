package controller

import (
	"context"
	"testing"
	"time"

	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Unit fixtures reproduce the real M5 quota recovery failure: Deployment has
// observed the new generation while still carrying an old ReplicaSet failure.
func TestReconcileReplicaFailureBelongsToCurrentTemplate(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		currentFailure, foreign, unobserved bool
		degraded                            bool
	}{
		{name: "old ReplicaSet failure must not fail new release"},
		{name: "current ReplicaSet failure is reported", currentFailure: true, degraded: true},
		{name: "foreign ReplicaSet cannot fail release", currentFailure: true, foreign: true},
		{name: "unobserved ReplicaSet cannot fail release", currentFailure: true, unobserved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := testApp()
			dep := readyDeployment(app)
			dep.UID = "deployment-uid"
			dep.OwnerReferences = []metav1.OwnerReference{{APIVersion: platform.GroupVersion.String(), Kind: "Application", Name: app.Name, UID: app.UID, Controller: ptr(true)}}
			dep.Status = appsv1.DeploymentStatus{ObservedGeneration: dep.Generation, UnavailableReplicas: 2, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue, Reason: "FailedCreate", Message: "old quota failure"}}}
			current := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: app.Namespace, Generation: 1, Labels: map[string]string{UIDLabel: string(app.UID)}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: dep.Name, UID: dep.UID, Controller: ptr(true)}}}, Spec: appsv1.ReplicaSetSpec{Replicas: ptr[int32](2), Selector: dep.Spec.Selector.DeepCopy(), Template: *dep.Spec.Template.DeepCopy()}, Status: appsv1.ReplicaSetStatus{ObservedGeneration: 1}}
			current.Spec.Template.Labels[appsv1.DefaultDeploymentUniqueLabelKey] = "current-hash"
			old := current.DeepCopy()
			old.Name = "old"
			old.Spec.Template.Annotations = map[string]string{RevisionAnnotation: "1"}
			old.Status.Conditions = []appsv1.ReplicaSetCondition{{Type: appsv1.ReplicaSetReplicaFailure, Status: corev1.ConditionTrue, Reason: "FailedCreate", Message: "old quota failure"}}
			if tc.currentFailure {
				current.Status.Conditions = []appsv1.ReplicaSetCondition{{Type: appsv1.ReplicaSetReplicaFailure, Status: corev1.ConditionTrue, Reason: "FailedCreate", Message: "current quota failure"}}
			}
			if tc.foreign {
				current.OwnerReferences[0].UID = "foreign"
			}
			if tc.unobserved {
				current.Status.ObservedGeneration = 0
			}
			scheme := runtime.NewScheme()
			_ = platform.AddToScheme(scheme)
			_ = appsv1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&platform.Application{}, &appsv1.Deployment{}).WithObjects(app, dep, old, current).Build()
			r := &ApplicationReconciler{Client: c, Scheme: scheme}
			if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(app)}); err != nil {
				t.Fatal(err)
			}
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(app), app); err != nil {
				t.Fatal(err)
			}
			condition := meta.FindStatusCondition(app.Status.Conditions, "Degraded")
			if condition == nil || (condition.Status == metav1.ConditionTrue) != tc.degraded {
				t.Fatalf("wrong failure attribution: %+v", condition)
			}
			if tc.degraded && condition.Message != "current quota failure" {
				t.Fatalf("wrong ReplicaSet evidence: %+v", condition)
			}
			if meta.IsStatusConditionTrue(app.Status.Conditions, "Ready") {
				t.Fatal("pending or failed rollout manufactured Ready")
			}
		})
	}
}

func TestPodTemplateBindsAcceptedRevision(t *testing.T) {
	app := testApp()
	dep := readyDeployment(app)
	if dep.Spec.Template.Annotations[RevisionAnnotation] != "2" {
		t.Fatal("Pod template must identify the accepted revision")
	}
	dep.Spec.Template.Annotations["user"] = "preserve"
	app.Spec.Revision = 3
	mutateDeployment(app, dep)
	if dep.Spec.Template.Annotations[RevisionAnnotation] != "3" || dep.Spec.Template.Annotations["user"] != "preserve" {
		t.Fatal("new revision must replace managed marker while preserving user metadata")
	}
}

func deadlineReplicaSet(dep *appsv1.Deployment, created time.Time) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: dep.Namespace, Generation: 1, CreationTimestamp: metav1.NewTime(created), Labels: map[string]string{UIDLabel: dep.Labels[UIDLabel]}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: dep.Name, UID: dep.UID, Controller: ptr(true)}}}, Spec: appsv1.ReplicaSetSpec{Replicas: ptr[int32](2), Selector: dep.Spec.Selector.DeepCopy(), Template: *dep.Spec.Template.DeepCopy()}, Status: appsv1.ReplicaSetStatus{ObservedGeneration: 1}}
}

// A timed-out Deployment condition can survive into a new accepted revision.
// These Unit fixtures keep observedGeneration current, as in the real rollback.
func TestReconcileDeadlineBelongsToCurrentTemplate(t *testing.T) {
	created := time.Unix(1700000000, 0)
	for _, tc := range []struct {
		name                                   string
		conditionOffset                        time.Duration
		missing, foreign, unobserved, degraded bool
	}{
		{name: "old timeout predates current ReplicaSet", conditionOffset: -time.Second},
		{name: "new rollout cannot time out in one second", conditionOffset: time.Second},
		{name: "current rollout timeout is retained", conditionOffset: 61 * time.Second, degraded: true},
		{name: "missing current ReplicaSet is not failure evidence", conditionOffset: 61 * time.Second, missing: true},
		{name: "foreign ReplicaSet is not timeout evidence", conditionOffset: 61 * time.Second, foreign: true},
		{name: "unobserved ReplicaSet is not timeout evidence", conditionOffset: 61 * time.Second, unobserved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := testApp()
			dep := readyDeployment(app)
			dep.UID = "deployment-uid"
			dep.OwnerReferences = []metav1.OwnerReference{{APIVersion: platform.GroupVersion.String(), Kind: "Application", Name: app.Name, UID: app.UID, Controller: ptr(true)}}
			dep.Status = appsv1.DeploymentStatus{ObservedGeneration: dep.Generation, UnavailableReplicas: 2, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded", Message: "timed out progressing", LastUpdateTime: metav1.NewTime(created.Add(tc.conditionOffset))}}}
			current := deadlineReplicaSet(dep, created)
			if tc.foreign {
				current.OwnerReferences[0].UID = "foreign"
			}
			if tc.unobserved {
				current.Status.ObservedGeneration = 0
			}
			scheme := runtime.NewScheme()
			_ = platform.AddToScheme(scheme)
			_ = appsv1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			objects := []client.Object{app, dep}
			if !tc.missing {
				objects = append(objects, current)
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&platform.Application{}, &appsv1.Deployment{}).WithObjects(objects...).Build()
			r := &ApplicationReconciler{Client: c, Scheme: scheme}
			if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(app)}); err != nil {
				t.Fatal(err)
			}
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(app), app); err != nil {
				t.Fatal(err)
			}
			condition := meta.FindStatusCondition(app.Status.Conditions, "Degraded")
			if condition == nil || (condition.Status == metav1.ConditionTrue) != tc.degraded {
				t.Fatalf("wrong timeout attribution: %+v", condition)
			}
			if tc.degraded && condition.Reason != "ProgressDeadlineExceeded" {
				t.Fatalf("current timeout was hidden: %+v", condition)
			}
			if meta.IsStatusConditionTrue(app.Status.Conditions, "Ready") {
				t.Fatal("pending/failed rollout manufactured Ready")
			}
		})
	}
}
