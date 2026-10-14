package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m3/agent"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func TestAgentCredentialProjectionAndLedgerSurviveReinstall(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme, rbacv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := &Provisioner{Client: c, Cluster: "hugofleet-a"}
	credentials := map[string][]byte{"ca.crt": []byte("unit CA"), "client.crt": []byte("unit cert"), "client.key": []byte("unit private key")}
	if err := p.InstallAgent(ctx, []string{"team-a", "team-b"}, "172.19.0.2:18443", credentials); err != nil {
		t.Fatal(err)
	}
	dep := &appsv1.Deployment{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: ControlNamespace, Name: "hugofleet-agent"}, dep); err != nil {
		t.Fatal(err)
	}
	items := dep.Spec.Template.Spec.Volumes[0].Secret.Items
	actual := map[string]string{}
	for _, item := range items {
		actual[item.Key] = item.Path
	}
	if len(actual) != 3 || actual["ca.crt"] != "ca.crt" || actual["client.crt"] != "hugofleet-a.crt" || actual["client.key"] != "hugofleet-a.key" {
		t.Fatal("Worker credentials do not match the Agent TLS reader's exact filenames")
	}
	ledger := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: ControlNamespace, Name: agent.LedgerName}, ledger); err != nil {
		t.Fatal(err)
	}
	ledger.Data = map[string]string{"ledger": "unit tombstone high water"}
	if err := c.Update(ctx, ledger); err != nil {
		t.Fatal(err)
	}
	if err := p.InstallAgent(ctx, []string{"team-a"}, "172.19.0.3:18443", credentials); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(ledger), ledger); err != nil || ledger.Data["ledger"] != "unit tombstone high water" {
		t.Fatal("Agent reinstall reset persistent command ledger")
	}
	ledger.Labels[ClusterLabel] = "foreign"
	if err := c.Update(ctx, ledger); err != nil {
		t.Fatal(err)
	}
	if err := p.InstallAgent(ctx, []string{"team-a"}, "172.19.0.3:18443", credentials); err == nil {
		t.Fatal("foreign ledger adopted")
	}
}

func TestProfileProofRejectsDriftAndUnapprovedTenant(t *testing.T) {
	ctx := context.Background()
	p := domain.TenantProfile{TenantID: "team-a", ClusterID: "hugofleet-a", Namespace: "hf-team-a", Hard: map[string]string{"requests.cpu": "1", "requests.memory": "1Gi", "limits.cpu": "3", "limits.memory": "2Gi", "pods": "8"}, Defaults: map[string]string{"requests.cpu": "100m", "requests.memory": "64Mi", "limits.cpu": "300m", "limits.memory": "128Mi"}}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{ManagedLabel: Manager, TenantLabel: p.TenantID, ClusterLabel: p.ClusterID}
	metadata := func(name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: p.Namespace, UID: "receipt-uid", Labels: labels}
	}
	cm := &corev1.ConfigMap{ObjectMeta: metadata(ReceiptName), Data: map[string]string{"profile.json": string(raw)}}
	hard := corev1.ResourceList{}
	for key, val := range p.Hard {
		hard[corev1.ResourceName(key)] = resource.MustParse(val)
	}
	quota := &corev1.ResourceQuota{ObjectMeta: metadata(QuotaName), Spec: corev1.ResourceQuotaSpec{Hard: hard}, Status: corev1.ResourceQuotaStatus{Hard: hard.DeepCopy()}}
	limits := &corev1.LimitRange{ObjectMeta: metadata(LimitsName), Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{Type: corev1.LimitTypeContainer, Max: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("512Mi")}, DefaultRequest: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("64Mi")}, Default: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("300m"), corev1.ResourceMemory: resource.MustParse("128Mi")}}}}}
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(quota).WithObjects(cm, quota, limits).Build()
	reader := &Reader{Client: c, Cluster: p.ClusterID, Tenants: map[string]bool{p.TenantID: true}}
	got, err := reader.ReadProfile(ctx, p.TenantID)
	if err != nil || got.BootstrapUID != "receipt-uid" {
		t.Fatalf("proof %+v %v", got, err)
	}
	if _, err = reader.ReadProfile(ctx, "team-b"); err == nil {
		t.Fatal("unapproved profile returned")
	}
	if err = c.Get(ctx, client.ObjectKeyFromObject(quota), quota); err != nil {
		t.Fatal(err)
	}
	quota.Spec.Scopes = []corev1.ResourceQuotaScope{corev1.ResourceQuotaScopeTerminating}
	if err = c.Update(ctx, quota); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ReadProfile(ctx, p.TenantID); err == nil {
		t.Fatal("scope weakened all-Pod quota")
	}
	quota.Spec.Scopes = nil
	quota.Spec.Hard[corev1.ResourceCPU] = resource.MustParse("10")
	if err = c.Update(ctx, quota); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ReadProfile(ctx, p.TenantID); err == nil {
		t.Fatal("resource profile drift accepted")
	}
}
