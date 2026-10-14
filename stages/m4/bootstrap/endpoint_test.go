package bootstrap

import (
	"context"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func endpointDeployment() *appsv1.Deployment {
	d := deployment(metav1.ObjectMeta{Name: "hugofleet-agent", Namespace: ControlNamespace, UID: "agent-uid", Labels: map[string]string{ManagedLabel: Manager, ClusterLabel: "hugofleet-a", TenantLabel: ""}, Annotations: map[string]string{"custom": "retained"}}, "hugofleet-agent", "hugofleet-agent:m4", []string{"--hub=172.18.0.2:18443", "--cluster=hugofleet-a", "--tenants=team-a,team-b", "--heartbeat=7s", "--reconnect-min=2s", "--reconnect-max=40s"}, 50, 64, 500, 256)
	d.Spec.Template.Annotations = map[string]string{"rollout": "3"}
	return d
}

func endpointClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func TestEndpointUpdatePreservesWorkerPolicyAndAgentSettings(t *testing.T) {
	ctx := context.Background()
	d := endpointDeployment()
	quota := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: QuotaName, Namespace: "hf-team-a", UID: "quota-uid"}, Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{corev1.ResourceRequestsCPU: resource.MustParse("2")}}}
	receipt := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ReceiptName, Namespace: "hf-team-a", UID: "receipt-uid"}, Data: map[string]string{"profile.json": "custom profile"}}
	ledger := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "hugofleet-agent-ledger", Namespace: ControlNamespace, UID: "ledger-uid"}, Data: map[string]string{"ledger": "tombstone/high-water"}}
	c := endpointClient(t, d, quota, receipt, ledger)
	before := &appsv1.Deployment{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(d), before); err != nil {
		t.Fatal(err)
	}
	p := &Provisioner{Client: c, Cluster: "hugofleet-a"}
	if err := p.UpdateAgentEndpoint(ctx, "172.18.0.3:18443"); err != nil {
		t.Fatal(err)
	}
	after := &appsv1.Deployment{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(d), after); err != nil {
		t.Fatal(err)
	}
	expected := before.DeepCopy()
	expected.Spec.Template.Spec.Containers[0].Args[0] = "--hub=172.18.0.3:18443"
	after.ResourceVersion = expected.ResourceVersion
	if !reflect.DeepEqual(after, expected) {
		t.Fatal("endpoint update changed fields other than the Hub argument")
	}
	for _, before := range []client.Object{quota, receipt, ledger} {
		after := before.DeepCopyObject().(client.Object)
		if err := c.Get(ctx, client.ObjectKeyFromObject(before), after); err != nil {
			t.Fatal(err)
		}
		after.SetResourceVersion(before.GetResourceVersion())
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("endpoint update changed policy/ledger %T", before)
		}
	}
}

func TestEndpointUpdateRefusesForeignOrAmbiguousAgent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*appsv1.Deployment)
	}{
		{"manager", func(d *appsv1.Deployment) { d.Labels[ManagedLabel] = "foreign" }},
		{"cluster", func(d *appsv1.Deployment) { d.Labels[ClusterLabel] = "hugofleet-b" }},
		{"tenant", func(d *appsv1.Deployment) { d.Labels[TenantLabel] = "team-a" }},
		{"helm", func(d *appsv1.Deployment) { d.Annotations["meta.helm.sh/release-name"] = "foreign" }},
		{"sidecar", func(d *appsv1.Deployment) {
			d.Spec.Template.Spec.Containers = append(d.Spec.Template.Spec.Containers, corev1.Container{Name: "sidecar"})
		}},
		{"container", func(d *appsv1.Deployment) { d.Spec.Template.Spec.Containers[0].Name = "foreign" }},
		{"missing-hub", func(d *appsv1.Deployment) { d.Spec.Template.Spec.Containers[0].Args[0] = "--version=m4" }},
		{"duplicate-hub", func(d *appsv1.Deployment) {
			d.Spec.Template.Spec.Containers[0].Args = append(d.Spec.Template.Spec.Containers[0].Args, "--hub=172.18.0.4:18443")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := endpointDeployment()
			tc.change(d)
			c := endpointClient(t, d)
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(d), d); err != nil {
				t.Fatal(err)
			}
			before := d.DeepCopy()
			if err := (&Provisioner{Client: c, Cluster: "hugofleet-a"}).UpdateAgentEndpoint(context.Background(), "172.18.0.3:18443"); err == nil {
				t.Fatal("foreign or ambiguous Agent updated")
			}
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(d), d); err != nil {
				t.Fatal(err)
			}
			d.ResourceVersion = before.ResourceVersion
			if !reflect.DeepEqual(d, before) {
				t.Fatal("refused update still changed the Deployment")
			}
		})
	}
	for _, endpoint := range []string{"203.0.113.1:18443", "172.18.0.3:443", "localhost:18443", "172.18.0.999:18443", "172.18.0.3"} {
		if err := (&Provisioner{Client: endpointClient(t, endpointDeployment()), Cluster: "hugofleet-a"}).UpdateAgentEndpoint(context.Background(), endpoint); err == nil {
			t.Fatalf("invalid endpoint accepted: %s", endpoint)
		}
	}
	if err := (&Provisioner{Client: endpointClient(t, endpointDeployment()), Cluster: "hugofleet-a", PrepareOnly: true}).UpdateAgentEndpoint(context.Background(), "172.18.0.3:18443"); err == nil {
		t.Fatal("direct endpoint update adopted Helm preparation mode")
	}
}

type concurrentEndpointClient struct{ client.Client }

func (c concurrentEndpointClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	latest := &appsv1.Deployment{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(obj), latest); err != nil {
		return err
	}
	latest.Spec.Template.Spec.Containers[0].Args[0] = "--hub=172.18.0.9:18443"
	if err := c.Update(ctx, latest); err != nil {
		return err
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func TestEndpointUpdateRejectsConcurrentDeploymentChange(t *testing.T) {
	c := endpointClient(t, endpointDeployment())
	err := (&Provisioner{Client: concurrentEndpointClient{c}, Cluster: "hugofleet-a"}).UpdateAgentEndpoint(context.Background(), "172.18.0.3:18443")
	if !apierrors.IsConflict(err) {
		t.Fatalf("expected optimistic-lock conflict, got %v", err)
	}
	d := &appsv1.Deployment{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: ControlNamespace, Name: "hugofleet-agent"}, d); err != nil {
		t.Fatal(err)
	}
	if d.Spec.Template.Spec.Containers[0].Args[0] != "--hub=172.18.0.9:18443" {
		t.Fatal("endpoint patch overwrote a concurrent change")
	}
}
