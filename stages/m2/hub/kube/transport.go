package kube

import (
	"context"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m1/controller"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m5/observability"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	authorization "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"sync"
)

const HubIDLabel = "platform.hugofleet.io/hub-application-id"

type Transport struct {
	c              client.Client
	auth           authorization.AuthorizationV1Interface
	accessMu       sync.Mutex
	accessVerified bool
	tenants        []string
}

func New(path, contextName string) (*Transport, error) {
	cfg, err := localConfig(path, contextName)
	if err != nil {
		return nil, err
	}
	return NewWithConfig(cfg)
}

// NewWithConfig is used only by a Worker Agent with its in-cluster ServiceAccount.
func NewWithConfig(cfg *rest.Config) (*Transport, error) {
	return NewForTenants(cfg, []string{domain.TenantID})
}

// NewForTenants takes a trusted deployment allowlist, never a browser value.
func NewForTenants(cfg *rest.Config, tenants []string) (*Transport, error) {
	if len(tenants) < 1 || len(tenants) > 20 {
		return nil, domain.Problem("INVALID_ARGUMENT", "1..20 approved tenant namespaces required")
	}
	seen := map[string]bool{}
	for _, tenant := range tenants {
		if err := domain.ValidateTenant(tenant); err != nil {
			return nil, err
		}
		if seen[tenant] {
			return nil, domain.Problem("INVALID_ARGUMENT", "duplicate tenant allowlist entry")
		}
		seen[tenant] = true
	}
	var err error
	cfg = rest.CopyConfig(cfg)
	cfg.Timeout = 5e9
	cfg.QPS = 10
	cfg.Burst = 15
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{platform.AddToScheme, appsv1.AddToScheme, corev1.AddToScheme} {
		if err = add(scheme); err != nil {
			return nil, err
		}
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	auth, err := authorization.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Transport{c: c, auth: auth, tenants: append([]string(nil), tenants...)}, nil
}
func (t *Transport) Ping(ctx context.Context) error {
	if err := t.verifyAccess(ctx); err != nil {
		return err
	}
	var apps platform.ApplicationList
	return t.c.List(ctx, &apps, client.InNamespace(domain.NamespaceForTenant(t.tenants[0])), client.Limit(1))
}
func key(a domain.Application) client.ObjectKey {
	return client.ObjectKey{Namespace: domain.NamespaceForTenant(a.TenantID), Name: a.Name}
}
func (t *Transport) allowed(a domain.Application) error {
	for _, tenant := range t.tenants {
		if a.TenantID == tenant {
			return nil
		}
	}
	return domain.Problem("PERMISSION_DENIED", "Worker tenant is outside the trusted allowlist")
}
func bound(a domain.Application, cr *platform.Application) error {
	if cr.Labels[HubIDLabel] != a.ID || (a.KubernetesUID != "" && string(cr.UID) != a.KubernetesUID) {
		return domain.Problem("OWNERSHIP_CONFLICT", "Application CR is not bound to this Hub application and UID")
	}
	if !cr.DeletionTimestamp.IsZero() {
		return domain.Problem("CONFLICT", "Application CR is deleting")
	}
	return nil
}
func (t *Transport) Apply(ctx context.Context, a domain.Application, spec platform.ApplicationSpec) (string, error) {
	if err := t.allowed(a); err != nil {
		return "", err
	}
	var cr platform.Application
	err := t.c.Get(ctx, key(a), &cr)
	if apierrors.IsNotFound(err) {
		if a.KubernetesUID != "" {
			return "", domain.Problem("APPLICATION_MISSING", "previously bound Application CR disappeared; refusing to silently recreate its UID")
		}
		cr = platform.Application{ObjectMeta: metav1.ObjectMeta{Name: a.Name, Namespace: key(a).Namespace, Labels: map[string]string{HubIDLabel: a.ID}, Annotations: observability.Annotations(ctx)}, Spec: spec}
		if err = t.c.Create(ctx, &cr); err != nil {
			return "", err
		}
		return string(cr.UID), nil
	}
	if err != nil {
		return "", err
	}
	if err = bound(a, &cr); err != nil {
		return "", err
	}
	if cr.Spec.Revision > spec.Revision {
		return "", domain.Problem("CONFLICT", "cluster revision is newer than accepted Hub intent")
	}
	if cr.Spec.Revision == spec.Revision {
		if !equality.Semantic.DeepEqual(cr.Spec, spec) {
			return "", domain.Problem("CONFLICT", "same revision has a different specification")
		}
		return string(cr.UID), nil
	}
	cr.Spec = spec
	for k, v := range observability.Annotations(ctx) {
		if cr.Annotations == nil {
			cr.Annotations = map[string]string{}
		}
		cr.Annotations[k] = v
	}
	if err = t.c.Update(ctx, &cr); err != nil {
		return "", err
	}
	return string(cr.UID), nil
}
func (t *Transport) Observe(ctx context.Context, a domain.Application, spec platform.ApplicationSpec) (ob domain.Observation, err error) {
	if err = t.allowed(a); err != nil {
		return ob, err
	}
	var cr platform.Application
	if err = t.c.Get(ctx, key(a), &cr); apierrors.IsNotFound(err) {
		return domain.Observation{ErrorCode: "APPLICATION_MISSING", Message: "Application CR is missing"}, nil
	} else if err != nil {
		return ob, err
	}
	if err = bound(a, &cr); err != nil {
		return ob, err
	}
	ob.UID = string(cr.UID)
	ob.ObservedRevision = cr.Status.ObservedRevision
	ob.Generation = cr.Generation
	ob.ObservedGeneration = cr.Status.ObservedGeneration
	ob.Conditions = cr.Status.Conditions
	if !equality.Semantic.DeepEqual(cr.Spec, spec) {
		return domain.Observation{ErrorCode: "SPEC_DRIFT", Message: "Application CR does not match the accepted revision snapshot"}, nil
	}
	ob.Applied = true
	degraded := meta.FindStatusCondition(cr.Status.Conditions, "Degraded")
	if degraded != nil && degraded.Status == metav1.ConditionTrue && degraded.ObservedGeneration == cr.Generation && cr.Status.ObservedGeneration == cr.Generation {
		ob.ErrorCode, ob.Message = degraded.Reason, degraded.Message
		return ob, nil
	}
	ready := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
	if ready == nil || ready.Status != metav1.ConditionTrue || ready.ObservedGeneration != cr.Generation || cr.Status.ObservedGeneration != cr.Generation || cr.Status.ObservedRevision != spec.Revision || cr.Status.Image != spec.Image {
		return ob, nil
	}
	var dep appsv1.Deployment
	var svc corev1.Service
	for _, obj := range []client.Object{&dep, &svc} {
		if err = t.c.Get(ctx, key(a), obj); apierrors.IsNotFound(err) {
			return ob, nil
		} else if err != nil {
			return ob, err
		}
		owner := metav1.GetControllerOf(obj)
		if owner == nil || owner.UID != cr.UID || owner.Kind != "Application" || owner.APIVersion != platform.GroupVersion.String() || obj.GetAnnotations()[controller.RevisionAnnotation] != strconv.FormatInt(spec.Revision, 10) {
			return ob, nil
		}
	}
	expected := controller.DesiredDeploymentSpec(&cr)
	for k, v := range dep.Spec.Template.Labels {
		if _, exists := expected.Template.Labels[k]; !exists {
			expected.Template.Labels[k] = v
		}
	}
	for k, v := range dep.Spec.Template.Annotations {
		if _, managed := expected.Template.Annotations[k]; !managed {
			expected.Template.Annotations[k] = v
		}
	}
	n := spec.Replicas
	if !equality.Semantic.DeepEqual(dep.Spec, expected) || dep.Status.ObservedGeneration < dep.Generation || dep.Status.Replicas != n || dep.Status.UpdatedReplicas != n || dep.Status.ReadyReplicas != n || dep.Status.AvailableReplicas != n || dep.Status.UnavailableReplicas != 0 {
		return ob, nil
	}
	if len(svc.Spec.Selector) != 1 || svc.Spec.Selector[controller.UIDLabel] != string(cr.UID) || len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != spec.Port || svc.Spec.Ports[0].TargetPort.StrVal != "http" {
		return ob, nil
	}
	ob.Ready = true
	return ob, nil
}
func (t *Transport) Delete(ctx context.Context, a domain.Application) (domain.Observation, error) {
	if err := t.allowed(a); err != nil {
		return domain.Observation{}, err
	}
	var cr platform.Application
	err := t.c.Get(ctx, key(a), &cr)
	if err == nil {
		if cr.Labels[HubIDLabel] != a.ID || (a.KubernetesUID != "" && string(cr.UID) != a.KubernetesUID) {
			return domain.Observation{}, domain.Problem("OWNERSHIP_CONFLICT", "refusing to delete a foreign Application CR")
		}
		uid := cr.UID
		if a.KubernetesUID == "" {
			// A prior create may have reached Kubernetes before the Hub crashed.
			// Keep the CR until its UID is durably bound for later GC checks.
			return domain.Observation{UID: string(uid)}, nil
		}
		policy := metav1.DeletePropagationBackground
		if cr.DeletionTimestamp.IsZero() {
			if err = t.c.Delete(ctx, &cr, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}, PropagationPolicy: &policy}); err != nil && !apierrors.IsNotFound(err) {
				return domain.Observation{}, err
			}
		}
		return domain.Observation{UID: string(uid)}, nil
	}
	if !apierrors.IsNotFound(err) {
		return domain.Observation{}, err
	}
	return t.DeletionStatus(ctx, a)
}

// DeletionStatus is read-only; snapshots must never perform a deletion.
func (t *Transport) DeletionStatus(ctx context.Context, a domain.Application) (domain.Observation, error) {
	if err := t.allowed(a); err != nil {
		return domain.Observation{}, err
	}
	var cr platform.Application
	err := t.c.Get(ctx, key(a), &cr)
	if err == nil {
		if cr.Spec.Revision > a.DesiredRevision {
			return domain.Observation{}, domain.Problem("STALE_REVISION", "delete precondition revision is older than Worker CR")
		}
		if cr.Labels[HubIDLabel] != a.ID || (a.KubernetesUID != "" && string(cr.UID) != a.KubernetesUID) {
			return domain.Observation{}, domain.Problem("OWNERSHIP_CONFLICT", "foreign Application identity")
		}
		return domain.Observation{UID: string(cr.UID)}, nil
	}
	if !apierrors.IsNotFound(err) {
		return domain.Observation{}, err
	}
	// Absence of CR alone is insufficient: verify its owned workload and Pods.
	for _, obj := range []client.Object{&appsv1.Deployment{}, &corev1.Service{}} {
		err = t.c.Get(ctx, key(a), obj)
		if err == nil {
			return domain.Observation{}, nil
		}
		if !apierrors.IsNotFound(err) {
			return domain.Observation{}, err
		}
	}
	{
		labels := client.MatchingLabels{controller.UIDLabel: a.KubernetesUID}
		if a.KubernetesUID == "" {
			// With no persisted UID and no CR, conservatively wait for any
			// remaining workload bearing this application's managed name.
			labels = client.MatchingLabels{"app.kubernetes.io/name": a.Name}
		}
		var pods corev1.PodList
		var sets appsv1.ReplicaSetList
		if err = t.c.List(ctx, &pods, client.InNamespace(key(a).Namespace), labels); err != nil {
			return domain.Observation{}, err
		}
		if err = t.c.List(ctx, &sets, client.InNamespace(key(a).Namespace), labels); err != nil {
			return domain.Observation{}, err
		}
		if len(pods.Items) > 0 || len(sets.Items) > 0 {
			return domain.Observation{}, nil
		}
	}
	return domain.Observation{Deleted: true}, nil
}
