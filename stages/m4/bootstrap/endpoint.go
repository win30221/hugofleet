package bootstrap

import (
	"context"
	"fmt"
	"net"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateAgentEndpoint changes only the connection address on an existing M4
// Agent. Recreating a Hub must not reprovision tenant admission policy or ledger.
func (p *Provisioner) UpdateAgentEndpoint(ctx context.Context, endpoint string) error {
	host, port, err := net.SplitHostPort(endpoint)
	ip := net.ParseIP(host)
	if err != nil || port != "18443" || ip == nil || !ip.IsPrivate() || p.PrepareOnly {
		return fmt.Errorf("M4 endpoint update requires a private Hub IP:18443")
	}
	d := &appsv1.Deployment{}
	if err = p.Client.Get(ctx, client.ObjectKey{Namespace: ControlNamespace, Name: "hugofleet-agent"}, d); err != nil {
		return err
	}
	if d.Labels[ManagedLabel] != Manager || d.Labels[ClusterLabel] != p.Cluster || d.Labels[TenantLabel] != "" || d.Annotations["meta.helm.sh/release-name"] != "" || d.Annotations["meta.helm.sh/release-namespace"] != "" || len(d.Spec.Template.Spec.Containers) != 1 || d.Spec.Template.Spec.Containers[0].Name != "hugofleet-agent" {
		return fmt.Errorf("refusing endpoint update on foreign Agent")
	}
	before := d.DeepCopy()
	count := 0
	for i, arg := range d.Spec.Template.Spec.Containers[0].Args {
		if strings.HasPrefix(arg, "--hub=") {
			d.Spec.Template.Spec.Containers[0].Args[i] = "--hub=" + endpoint
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("expected exactly one Agent Hub address")
	}
	return p.Client.Patch(ctx, d, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{}))
}
