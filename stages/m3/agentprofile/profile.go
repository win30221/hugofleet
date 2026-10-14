// Package agentprofile performs read-only tenant bootstrap verification with the
// ordinary Agent's limited ServiceAccount. It has no provisioning functions.
package agentprofile

import (
	"context"
	"encoding/json"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ReceiptName = "hugofleet-tenant-bootstrap"
const QuotaName = "hugofleet-quota"
const LimitsName = "hugofleet-limits"
const ControlNamespace = "hf-system"
const ManagedLabel = "app.kubernetes.io/managed-by"
const TenantLabel = "platform.hugofleet.io/tenant"
const ClusterLabel = "platform.hugofleet.io/cluster"
const Manager = "hugofleet-bootstrap"

type Reader struct {
	Client  client.Client
	Cluster string
	Tenants map[string]bool
}

func (r *Reader) ReadProfile(ctx context.Context, tenant string) (domain.TenantProfile, error) {
	var p domain.TenantProfile
	if !r.Tenants[tenant] || domain.ValidateTenant(tenant) != nil {
		return p, domain.Problem("PERMISSION_DENIED", "tenant bootstrap is outside the Worker allowlist")
	}
	namespace := domain.NamespaceForTenant(tenant)
	cm := &corev1.ConfigMap{}
	quota := &corev1.ResourceQuota{}
	limits := &corev1.LimitRange{}
	for _, item := range []struct {
		name   string
		object client.Object
	}{{ReceiptName, cm}, {QuotaName, quota}, {LimitsName, limits}} {
		if err := r.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: item.name}, item.object); err != nil {
			return p, err
		}
		labels := item.object.GetLabels()
		if labels[ManagedLabel] != Manager || labels[TenantLabel] != tenant || labels[ClusterLabel] != r.Cluster {
			return p, domain.Problem("OWNERSHIP_CONFLICT", "tenant bootstrap resource has a different owner")
		}
	}
	if err := json.Unmarshal([]byte(cm.Data["profile.json"]), &p); err != nil {
		return p, domain.Problem("INVALID_ARGUMENT", "invalid protected bootstrap receipt")
	}
	p.BootstrapUID = string(cm.UID)
	if p.TenantID != tenant || p.Namespace != namespace || p.ClusterID != r.Cluster {
		return p, domain.Problem("OWNERSHIP_CONFLICT", "bootstrap receipt identity mismatch")
	}
	if err := p.Validate(); err != nil {
		return p, err
	}
	if len(quota.Spec.Hard) != 5 || len(quota.Spec.Scopes) != 0 || quota.Spec.ScopeSelector != nil || len(limits.Spec.Limits) != 1 || limits.Spec.Limits[0].Type != corev1.LimitTypeContainer {
		return p, domain.Problem("OWNERSHIP_CONFLICT", "quota/limits do not match the approved profile")
	}
	for key, val := range p.Hard {
		expected := resource.MustParse(val)
		actual, ok := quota.Spec.Hard[corev1.ResourceName(key)]
		if !ok || actual.Cmp(expected) != 0 {
			return p, domain.Problem("OWNERSHIP_CONFLICT", "ResourceQuota changed outside its bootstrap profile")
		}
		observed, ok := quota.Status.Hard[corev1.ResourceName(key)]
		if !ok || observed.Cmp(actual) != 0 {
			return p, domain.Problem("UNAVAILABLE", "ResourceQuota admission controller has not observed the profile yet")
		}
	}
	rule := limits.Spec.Limits[0]
	for key, val := range map[corev1.ResourceName]string{corev1.ResourceCPU: "1", corev1.ResourceMemory: "512Mi"} {
		expected := resource.MustParse(val)
		actual, ok := rule.Max[key]
		if !ok || actual.Cmp(expected) != 0 {
			return p, domain.Problem("OWNERSHIP_CONFLICT", "LimitRange maximum does not match the platform policy")
		}
	}
	for key, val := range p.Defaults {
		expected := resource.MustParse(val)
		var actual resource.Quantity
		var ok bool
		switch key {
		case "requests.cpu":
			actual, ok = rule.DefaultRequest[corev1.ResourceCPU]
		case "requests.memory":
			actual, ok = rule.DefaultRequest[corev1.ResourceMemory]
		case "limits.cpu":
			actual, ok = rule.Default[corev1.ResourceCPU]
		case "limits.memory":
			actual, ok = rule.Default[corev1.ResourceMemory]
		}
		if !ok || actual.Cmp(expected) != 0 {
			return p, domain.Problem("OWNERSHIP_CONFLICT", "LimitRange defaults changed outside its bootstrap profile")
		}
	}
	return p, nil
}
