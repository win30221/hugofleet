package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m3/agent"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func ptr[T any](v T) *T { return &v }
func DefaultProfile(tenant, cluster string) domain.TenantProfile {
	return domain.TenantProfile{TenantID: tenant, ClusterID: cluster, Namespace: domain.NamespaceForTenant(tenant), Hard: map[string]string{"requests.cpu": "1", "requests.memory": "1Gi", "limits.cpu": "3", "limits.memory": "2Gi", "pods": "8"}, Defaults: map[string]string{"requests.cpu": "100m", "requests.memory": "64Mi", "limits.cpu": "300m", "limits.memory": "128Mi"}}
}

type Provisioner struct {
	// M5 leaves workload/RBAC ownership to Helm. Security material remains in
	// the controlled local bootstrap and survives Helm uninstall.
	PrepareOnly bool
	Client      client.Client
	Cluster     string
}

func (p *Provisioner) metadata(name, namespace, tenant string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{ManagedLabel: Manager, TenantLabel: tenant, ClusterLabel: p.Cluster}}
}
func (p *Provisioner) apply(ctx context.Context, obj client.Object) error {
	old := obj.DeepCopyObject().(client.Object)
	err := p.Client.Get(ctx, client.ObjectKeyFromObject(obj), old)
	if apierrors.IsNotFound(err) {
		return p.Client.Create(ctx, obj)
	}
	if err != nil {
		return err
	}
	labels := old.GetLabels()
	expected := obj.GetLabels()
	if labels[ManagedLabel] != Manager || labels[ClusterLabel] != p.Cluster || labels[TenantLabel] != expected[TenantLabel] {
		return fmt.Errorf("refusing to adopt foreign %T %s/%s", obj, obj.GetNamespace(), obj.GetName())
	}
	if _, ok := obj.(*corev1.Namespace); ok {
		for key, val := range expected {
			labels[key] = val
		}
		old.SetLabels(labels)
		return p.Client.Update(ctx, old)
	}
	// Only this bootstrap's named resources may be updated. Ordinary Agents cannot
	// call this tool or mutate any of these admission/RBAC resources.
	obj.SetResourceVersion(old.GetResourceVersion())
	obj.SetUID(old.GetUID())
	obj.SetCreationTimestamp(old.GetCreationTimestamp())
	return p.Client.Update(ctx, obj)
}
func (p *Provisioner) namespace(ctx context.Context, name, tenant string) error {
	metadata := p.metadata(name, "", tenant)
	metadata.Labels["pod-security.kubernetes.io/enforce"] = "restricted"
	metadata.Labels["pod-security.kubernetes.io/enforce-version"] = "v1.37"
	return p.apply(ctx, &corev1.Namespace{ObjectMeta: metadata})
}
func (p *Provisioner) ProvisionTenant(ctx context.Context, profile domain.TenantProfile) error {
	profile.BootstrapUID = "provisioning"
	if err := profile.Validate(); err != nil {
		return err
	}
	if profile.ClusterID != p.Cluster {
		return fmt.Errorf("tenant profile belongs to another Worker")
	}
	profile.BootstrapUID = ""
	namespace, tenant := profile.Namespace, profile.TenantID
	if err := p.namespace(ctx, namespace, tenant); err != nil {
		return err
	}
	hard := corev1.ResourceList{}
	for key, val := range profile.Hard {
		hard[corev1.ResourceName(key)] = resource.MustParse(val)
	}
	objects := []client.Object{
		&corev1.ResourceQuota{ObjectMeta: p.metadata(QuotaName, namespace, tenant), Spec: corev1.ResourceQuotaSpec{Hard: hard}},
		&corev1.LimitRange{ObjectMeta: p.metadata(LimitsName, namespace, tenant), Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{Type: corev1.LimitTypeContainer, Min: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("16Mi")}, Max: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("512Mi")}, DefaultRequest: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(profile.Defaults["requests.cpu"]), corev1.ResourceMemory: resource.MustParse(profile.Defaults["requests.memory"])}, Default: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(profile.Defaults["limits.cpu"]), corev1.ResourceMemory: resource.MustParse(profile.Defaults["limits.memory"])}}}}},
		&corev1.ServiceAccount{ObjectMeta: p.metadata("hugofleet-operator", namespace, tenant)},
		&rbacv1.Role{ObjectMeta: p.metadata("hugofleet-operator", namespace, tenant), Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"create", "get", "list", "patch", "update", "watch"}},
			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"create", "get", "list", "patch", "update", "watch"}},
			{APIGroups: []string{"apps"}, Resources: []string{"replicasets"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"platform.hugofleet.io"}, Resources: []string{"applications"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"platform.hugofleet.io"}, Resources: []string{"applications/finalizers"}, Verbs: []string{"update"}},
			{APIGroups: []string{"platform.hugofleet.io"}, Resources: []string{"applications/status"}, Verbs: []string{"get", "patch", "update"}},
		}},
		&rbacv1.RoleBinding{ObjectMeta: p.metadata("hugofleet-operator", namespace, tenant), Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "hugofleet-operator", Namespace: namespace}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "hugofleet-operator"}},
		&rbacv1.Role{ObjectMeta: p.metadata("hugofleet-agent", namespace, tenant), Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"platform.hugofleet.io"}, Resources: []string{"applications"}, Verbs: []string{"get", "list", "create", "update", "delete"}},
			{APIGroups: []string{"apps"}, Resources: []string{"deployments", "replicasets"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{""}, Resources: []string{"services", "pods"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{""}, Resources: []string{"resourcequotas"}, ResourceNames: []string{QuotaName}, Verbs: []string{"get"}},
			{APIGroups: []string{""}, Resources: []string{"limitranges"}, ResourceNames: []string{LimitsName}, Verbs: []string{"get"}},
			{APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{ReceiptName}, Verbs: []string{"get"}},
		}},
		&rbacv1.RoleBinding{ObjectMeta: p.metadata("hugofleet-agent", namespace, tenant), Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "hugofleet-agent", Namespace: ControlNamespace}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "hugofleet-agent"}},
		&networkingv1.NetworkPolicy{ObjectMeta: p.metadata("hugofleet-tenant-ingress", namespace, tenant), Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}, Ingress: []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{}}}}}}},
		deployment(p.metadata("hugofleet-operator", namespace, tenant), "hugofleet-operator", "hugofleet-operator:m1", []string{"--namespace=" + namespace, "--health-probe-bind-address=:8081"}, 100, 96, 500, 256),
	}
	operator := objects[len(objects)-1].(*appsv1.Deployment)
	operator.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: "health", ContainerPort: 8081}}
	operator.Spec.Template.Spec.Containers[0].ReadinessProbe = probe("/readyz", "health", 2)
	operator.Spec.Template.Spec.Containers[0].LivenessProbe = probe("/healthz", "health", 5)
	for _, obj := range objects {
		if p.PrepareOnly {
			switch obj.(type) {
			case *corev1.ServiceAccount, *rbacv1.Role, *rbacv1.RoleBinding, *appsv1.Deployment:
				continue
			}
		}
		if err := p.apply(ctx, obj); err != nil {
			return err
		}
	}
	if p.PrepareOnly {
		return nil
	}
	// Receipt is written last: partial RBAC/admission setup cannot activate a tenant.
	raw, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return p.apply(ctx, &corev1.ConfigMap{ObjectMeta: p.metadata(ReceiptName, namespace, tenant), Data: map[string]string{"profile.json": string(raw)}})
}
func deployment(metadata metav1.ObjectMeta, sa, image string, args []string, requestCPU, requestMemory, limitCPU, limitMemory int64) *appsv1.Deployment {
	labels := map[string]string{"app.kubernetes.io/name": metadata.Name}
	return &appsv1.Deployment{ObjectMeta: metadata, Spec: appsv1.DeploymentSpec{Replicas: ptr(int32(1)), Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{ServiceAccountName: sa, TerminationGracePeriodSeconds: ptr(int64(15)), SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: ptr(true), RunAsUser: ptr(int64(65532)), RunAsGroup: ptr(int64(65532)), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, Containers: []corev1.Container{{Name: sa, Image: image, ImagePullPolicy: corev1.PullIfNotPresent, Args: args, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: *resource.NewMilliQuantity(requestCPU, resource.DecimalSI), corev1.ResourceMemory: *resource.NewQuantity(requestMemory<<20, resource.BinarySI)}, Limits: corev1.ResourceList{corev1.ResourceCPU: *resource.NewMilliQuantity(limitCPU, resource.DecimalSI), corev1.ResourceMemory: *resource.NewQuantity(limitMemory<<20, resource.BinarySI)}}, SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: ptr(false), ReadOnlyRootFilesystem: ptr(true), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}}}}}}}
}
func probe(path, port string, period int32) *corev1.Probe {
	return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: path, Port: intstr.FromString(port)}}, PeriodSeconds: period}
}
func (p *Provisioner) InstallAgent(ctx context.Context, tenants []string, endpoint string, certs map[string][]byte) error {
	if len(tenants) < 1 || len(tenants) > 20 {
		return fmt.Errorf("bounded tenant allowlist required")
	}
	host, port, err := net.SplitHostPort(endpoint)
	ip := net.ParseIP(host)
	if err != nil || port == "" || ip == nil || !ip.IsPrivate() {
		return fmt.Errorf("Agent endpoint must be the resolved Hub Docker network IP:port")
	}
	seen := map[string]bool{}
	for _, tenant := range tenants {
		if err = domain.ValidateTenant(tenant); err != nil {
			return err
		}
		if seen[tenant] {
			return fmt.Errorf("duplicate tenant allowlist")
		}
		seen[tenant] = true
	}
	if len(certs) != 3 || len(certs["ca.crt"]) == 0 || len(certs["client.crt"]) == 0 || len(certs["client.key"]) == 0 {
		return fmt.Errorf("exact Worker-only credential set required")
	}
	if err = p.namespace(ctx, ControlNamespace, ""); err != nil {
		return err
	}
	objects := []client.Object{
		&corev1.ServiceAccount{ObjectMeta: p.metadata("hugofleet-agent", ControlNamespace, "")},
		&corev1.Secret{ObjectMeta: p.metadata("hugofleet-agent-tls", ControlNamespace, ""), Type: corev1.SecretTypeOpaque, Data: certs},
		&rbacv1.Role{ObjectMeta: p.metadata("hugofleet-agent-ledger", ControlNamespace, ""), Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{agent.LedgerName}, Verbs: []string{"get", "update"}}}},
		&rbacv1.RoleBinding{ObjectMeta: p.metadata("hugofleet-agent-ledger", ControlNamespace, ""), Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "hugofleet-agent", Namespace: ControlNamespace}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "hugofleet-agent-ledger"}},
	}
	for _, obj := range objects {
		if p.PrepareOnly {
			switch obj.(type) {
			case *corev1.ServiceAccount, *rbacv1.Role, *rbacv1.RoleBinding:
				continue
			}
		}
		if err = p.apply(ctx, obj); err != nil {
			return err
		}
	}
	// Never reset the high-water ledger on reinstall.
	ledger := &corev1.ConfigMap{}
	err = p.Client.Get(ctx, client.ObjectKey{Namespace: ControlNamespace, Name: agent.LedgerName}, ledger)
	if apierrors.IsNotFound(err) {
		if err = p.apply(ctx, &corev1.ConfigMap{ObjectMeta: p.metadata(agent.LedgerName, ControlNamespace, "")}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if ledger.Labels[ManagedLabel] != Manager || ledger.Labels[ClusterLabel] != p.Cluster {
		return fmt.Errorf("foreign Agent ledger")
	}
	if p.PrepareOnly {
		return nil
	}
	dep := deployment(p.metadata("hugofleet-agent", ControlNamespace, ""), "hugofleet-agent", "hugofleet-agent:m4", []string{"--hub=" + endpoint, "--cluster=" + p.Cluster, "--certs=/certs", "--version=m4", "--ledger-namespace=" + ControlNamespace, "--tenants=" + strings.Join(tenants, ",")}, 50, 64, 500, 256)
	dep.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "certs", MountPath: "/certs", ReadOnly: true}}
	dep.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "certs", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "hugofleet-agent-tls", DefaultMode: ptr(int32(0444)), Items: []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}, {Key: "client.crt", Path: p.Cluster + ".crt"}, {Key: "client.key", Path: p.Cluster + ".key"}}}}}}
	return p.apply(ctx, dep)
}

// FinalizeHelmTenant writes the profile receipt only after the exact Helm
// release's restricted components exist and have completed real rollouts.
func (p *Provisioner) FinalizeHelmTenant(ctx context.Context, profile domain.TenantProfile) error {
	if !p.PrepareOnly || profile.ClusterID != p.Cluster {
		return fmt.Errorf("Helm preparation mode and matching cluster required")
	}
	profile.BootstrapUID = "provisioning"
	if err := profile.Validate(); err != nil {
		return err
	}
	profile.BootstrapUID = ""
	ns := profile.Namespace
	objects := []client.Object{
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-operator", Namespace: ns}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-operator", Namespace: ns}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-operator", Namespace: ns}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-agent", Namespace: ns}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-agent", Namespace: ns}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-operator", Namespace: ns}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-agent", Namespace: ControlNamespace}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-agent-ledger", Namespace: ControlNamespace}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-agent-ledger", Namespace: ControlNamespace}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-agent", Namespace: ControlNamespace}},
	}
	for _, obj := range objects {
		if err := p.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			return err
		}
		if obj.GetLabels()[ManagedLabel] != "Helm" || obj.GetLabels()[ClusterLabel] != p.Cluster || obj.GetAnnotations()["meta.helm.sh/release-name"] != "hugofleet-worker" || obj.GetAnnotations()["meta.helm.sh/release-namespace"] != ControlNamespace {
			return fmt.Errorf("foreign or incomplete Helm Worker component %s/%s", obj.GetNamespace(), obj.GetName())
		}
		if d, ok := obj.(*appsv1.Deployment); ok {
			if d.Status.ObservedGeneration != d.Generation || d.Status.ReadyReplicas != 1 || d.Status.UpdatedReplicas != 1 || d.Status.Replicas != 1 || d.Status.AvailableReplicas != 1 {
				return fmt.Errorf("Helm Worker component has not completed its rollout")
			}
		}
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return p.apply(ctx, &corev1.ConfigMap{ObjectMeta: p.metadata(ReceiptName, ns, profile.TenantID), Data: map[string]string{"profile.json": string(raw)}})
}
