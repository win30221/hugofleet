package controller

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"go.opentelemetry.io/otel/attribute"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"log/slog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ApplicationReconciler struct {
	ReconcileError func()
	client.Client
	Scheme *runtime.Scheme
}

type ownershipConflict struct{ message string }

func (e *ownershipConflict) Error() string { return e.message }

func checkOwner(app *platform.Application, obj client.Object) error {
	if obj.GetResourceVersion() == "" {
		return nil
	}
	owner := metav1.GetControllerOf(obj)
	if owner == nil || owner.UID != app.UID || owner.APIVersion != platform.GroupVersion.String() || owner.Kind != "Application" || owner.Name != app.Name {
		return &ownershipConflict{fmt.Sprintf("%T %s/%s is not controlled by Application UID %s", obj, obj.GetNamespace(), obj.GetName(), app.UID)}
	}
	return nil
}

// +kubebuilder:rbac:groups=platform.hugofleet.io,resources=applications,verbs=get;list;watch,namespace=hf-demo
// +kubebuilder:rbac:groups=platform.hugofleet.io,resources=applications/status,verbs=get;update;patch,namespace=hf-demo
// +kubebuilder:rbac:groups=platform.hugofleet.io,resources=applications/finalizers,verbs=update,namespace=hf-demo
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch,namespace=hf-demo
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch,namespace=hf-demo
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch,namespace=hf-demo

// Reconcile continuously maintains one Application's owned workload.
func (r *ApplicationReconciler) Reconcile(parent context.Context, req ctrl.Request) (result ctrl.Result, returnErr error) {
	defer func() {
		if returnErr != nil && r.ReconcileError != nil {
			r.ReconcileError()
		}
	}()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	var app platform.Application
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	ctx = observability.Extract(ctx, app.Annotations["platform.hugofleet.io/traceparent"], app.Annotations["platform.hugofleet.io/tracestate"])
	ctx, span := observability.Tracer().Start(ctx, "operator.reconcile")
	defer span.End()
	requestID, operationID := app.Annotations["platform.hugofleet.io/request-id"], app.Annotations["platform.hugofleet.io/operation-id"]
	clusterID := app.Annotations["platform.hugofleet.io/cluster-id"]
	tenantID := strings.TrimPrefix(app.Namespace, "hf-")
	appID := app.Labels["platform.hugofleet.io/hub-application-id"]
	span.SetAttributes(attribute.String("request_id", requestID), attribute.String("operation_id", operationID), attribute.String("cluster_id", clusterID), attribute.String("tenant_id", tenantID), attribute.String("app_id", appID))
	defer func() {
		if span.IsRecording() {
			attrs := observability.LogAttributes(observability.Correlation{RequestID: requestID, OperationID: operationID, ClusterID: clusterID, TenantID: tenantID, AppID: appID})
			attrs = append(attrs, "trace_id", span.SpanContext().TraceID().String(), "failed", returnErr != nil)
			slog.Info("Application reconcile completed", attrs...)
		}
	}()
	if !app.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	if problems := validation.IsDNS1123Label(app.Name); len(problems) != 0 {
		return r.problem(ctx, &app, "InvalidSpec", "application name must be a DNS label of at most 63 characters", false)
	}
	if err := app.Spec.Validate(); err != nil {
		return r.problem(ctx, &app, "InvalidSpec", err.Error(), false)
	}
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Namespace}}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Namespace}}
	// Preflight both names before writing either; recheck ownership inside CreateOrUpdate too.
	for _, obj := range []client.Object{dep, svc} {
		err := r.Get(ctx, req.NamespacedName, obj)
		if err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil {
			if err := checkOwner(&app, obj); err != nil {
				return r.problem(ctx, &app, "OwnershipConflict", err.Error(), true)
			}
		}
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		if err := checkOwner(&app, dep); err != nil {
			return err
		}
		mutateDeployment(&app, dep)
		return controllerutil.SetControllerReference(&app, dep, r.Scheme)
	})
	if err == nil {
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
			if err := checkOwner(&app, svc); err != nil {
				return err
			}
			mutateService(&app, svc)
			return controllerutil.SetControllerReference(&app, svc, r.Scheme)
		})
	}
	if err != nil {
		var conflict *ownershipConflict
		if errors.As(err, &conflict) {
			return r.problem(ctx, &app, "OwnershipConflict", err.Error(), true)
		}
		_, statusErr := r.problem(ctx, &app, "ReconcileError", err.Error(), true)
		return ctrl.Result{}, errors.Join(err, statusErr)
	}
	var sets appsv1.ReplicaSetList
	for _, condition := range dep.Status.Conditions {
		failure := condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue
		deadline := condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse && condition.Reason == "ProgressDeadlineExceeded"
		if failure || deadline {
			if err := r.List(ctx, &sets, client.InNamespace(app.Namespace), client.MatchingLabels{UIDLabel: string(app.UID)}); err != nil {
				return ctrl.Result{}, err
			}
			break
		}
	}
	status := rolloutStatus(&app, dep, sets.Items...)
	if err := r.patchStatus(ctx, &app, status); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func rolloutStatus(app *platform.Application, dep *appsv1.Deployment, sets ...appsv1.ReplicaSet) platform.ApplicationStatus {
	status := *app.Status.DeepCopy()
	status.ObservedGeneration = app.Generation
	status.ReadyReplicas, status.AvailableReplicas = dep.Status.ReadyReplicas, dep.Status.AvailableReplicas
	ready, progressing, degraded := false, true, false
	reason, message := "Reconciling", "Waiting for Deployment controller to observe the desired configuration"
	// Build the same managed projection while preserving metadata that does not affect rollout.
	expected := dep.DeepCopy()
	mutateDeployment(app, expected)
	matches := equality.Semantic.DeepEqual(dep.Spec, expected.Spec) && dep.Labels[UIDLabel] == string(app.UID) && dep.Annotations[RevisionAnnotation] == strconv.FormatInt(app.Spec.Revision, 10)
	if matches && dep.Status.ObservedGeneration >= dep.Generation {
		status.ObservedRevision, status.Image = app.Spec.Revision, app.Spec.Image
		reason, message = "RolloutInProgress", "Waiting for all desired replicas to be updated, ready and available"
		n := app.Spec.Replicas
		if dep.Status.UpdatedReplicas == n && dep.Status.Replicas == n && dep.Status.ReadyReplicas == n && dep.Status.AvailableReplicas == n && dep.Status.UnavailableReplicas == 0 {
			ready, progressing, reason, message = true, false, "RolloutComplete", "Desired revision is fully available"
		} else {
			current := currentReplicaSet(dep, sets)
			for _, c := range dep.Status.Conditions {
				if currentProgressDeadline(dep, current, c) {
					progressing, degraded, reason, message = false, true, c.Reason, c.Message
				}
			}
			if failure := currentReplicaFailure(dep, sets); failure != nil {
				progressing, degraded, reason, message = false, true, failure.Reason, failure.Message
			}
		}
	}
	setConditions(&status, app.Generation, ready, progressing, degraded, reason, message)
	return status
}

// Deployment ReplicaFailure may originate from an old ReplicaSet even after
// observedGeneration advances. Attribute failure only to the current template.
func currentReplicaFailure(dep *appsv1.Deployment, sets []appsv1.ReplicaSet) *appsv1.ReplicaSetCondition {
	current := currentReplicaSet(dep, sets)
	if current == nil {
		return nil
	}
	for i := range current.Status.Conditions {
		condition := &current.Status.Conditions[i]
		if condition.Type == appsv1.ReplicaSetReplicaFailure && condition.Status == corev1.ConditionTrue {
			return condition
		}
	}
	return nil
}

func currentReplicaSet(dep *appsv1.Deployment, sets []appsv1.ReplicaSet) *appsv1.ReplicaSet {
	var current *appsv1.ReplicaSet
	want := dep.Spec.Template.DeepCopy()
	delete(want.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
	for i := range sets {
		rs := &sets[i]
		owner := metav1.GetControllerOf(rs)
		if dep.UID == "" || owner == nil || owner.UID != dep.UID || owner.Kind != "Deployment" || owner.APIVersion != "apps/v1" || rs.Namespace != dep.Namespace || !rs.DeletionTimestamp.IsZero() {
			continue
		}
		template := rs.Spec.Template.DeepCopy()
		delete(template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
		if !equality.Semantic.DeepEqual(want, template) {
			continue
		}
		if current == nil || rs.CreationTimestamp.Before(&current.CreationTimestamp) || (rs.CreationTimestamp.Equal(&current.CreationTimestamp) && rs.Name < current.Name) {
			current = rs
		}
	}
	if current == nil || current.Status.ObservedGeneration < current.Generation || current.Spec.Replicas == nil || *current.Spec.Replicas == 0 {
		return nil
	}
	return current
}

// A stale Progressing=False condition can also outlive its rollout. A current
// ReplicaSet must have existed for the configured deadline when the Deployment
// controller wrote that condition; observedGeneration alone is insufficient.
func currentProgressDeadline(dep *appsv1.Deployment, current *appsv1.ReplicaSet, c appsv1.DeploymentCondition) bool {
	if c.Type != appsv1.DeploymentProgressing || c.Status != corev1.ConditionFalse || c.Reason != "ProgressDeadlineExceeded" || current == nil || current.CreationTimestamp.IsZero() || dep.Spec.ProgressDeadlineSeconds == nil || *dep.Spec.ProgressDeadlineSeconds <= 0 {
		return false
	}
	earliest := current.CreationTimestamp.Time.Add(time.Duration(*dep.Spec.ProgressDeadlineSeconds) * time.Second)
	return !c.LastUpdateTime.Time.Before(earliest)
}

func setConditions(status *platform.ApplicationStatus, generation int64, ready, progressing, degraded bool, reason, message string) {
	for _, c := range []struct {
		name  string
		value bool
	}{{"Ready", ready}, {"Progressing", progressing}, {"Degraded", degraded}} {
		state := metav1.ConditionFalse
		if c.value {
			state = metav1.ConditionTrue
		}
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{Type: c.name, Status: state, ObservedGeneration: generation, Reason: reason, Message: message})
	}
}

func (r *ApplicationReconciler) problem(ctx context.Context, app *platform.Application, reason, message string, retry bool) (ctrl.Result, error) {
	status := *app.Status.DeepCopy()
	status.ObservedGeneration = app.Generation
	status.ReadyReplicas, status.AvailableReplicas = 0, 0
	setConditions(&status, app.Generation, false, false, true, reason, message)
	if err := r.patchStatus(ctx, app, status); err != nil {
		return ctrl.Result{}, err
	}
	result := ctrl.Result{}
	if retry {
		result.RequeueAfter = 10 * time.Second
	}
	return result, nil
}

func (r *ApplicationReconciler) patchStatus(ctx context.Context, app *platform.Application, status platform.ApplicationStatus) error {
	if equality.Semantic.DeepEqual(app.Status, status) {
		return nil
	}
	before := app.DeepCopy()
	app.Status = status
	return r.Status().Patch(ctx, app, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{}))
}

func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&platform.Application{}).Owns(&appsv1.Deployment{}).Owns(&corev1.Service{}).Complete(r)
}
