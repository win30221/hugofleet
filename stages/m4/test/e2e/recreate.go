//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m4/bootstrap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *suite) recreatePreservesPolicy() {
	s.t.Helper()
	root := os.Getenv("HF_ROOT")
	state := os.Getenv("HF_" + strings.ToUpper(s.stage) + "_STATE")
	w := s.workers[s.A]
	run := func(name string, args ...string) []byte {
		s.t.Helper()
		ctx, cancel := context.WithTimeout(s.ctx, 6*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			s.t.Fatalf("owned recreate regression command: %v %s", err, out)
		}
		return out
	}
	args := []string{"--stage=" + s.stage, "--cluster=" + s.A, "--kubeconfig=" + w.Kubeconfig, "--tenant=team-a", "--requests-cpu=2"}
	run(filepath.Join(root, "bin/bootstrap-worker"), append(args, "--action=tenant")...)
	if s.stage == "m5" {
		run(filepath.Join(root, "bin/bootstrap-worker"), append(args, "--action=receipt")...)
		run(filepath.Join(root, ".tools/bin/helm"), "upgrade", "hugofleet-worker", filepath.Join(root, "stages/m5/chart"), "--namespace", "hf-system", "--kubeconfig", w.Kubeconfig, "--kube-context", "kind-"+s.A, "--reset-then-reuse-values", "--set", "rollout=3,heartbeatSeconds=4,reconnectMinSeconds=2,reconnectMaxSeconds=40", "--wait=watcher", "--timeout=5m")
		// Pod readiness does not imply the replacement Agent has re-established
		// its authenticated channel. Tenant activation requires that real channel.
		eventually(s.t, s.ctx, 2*time.Minute, "real Agent reconnect after custom Helm upgrade", func() bool {
			var clusters []domain.Cluster
			s.api("admin", "GET", "/api/v1/clusters", "", nil, 200, &clusters)
			for _, c := range clusters {
				if c.ID == s.A {
					return c.State == "CONNECTED" && c.AgentVersion == "m5"
				}
			}
			return false
		})
	}
	eventually(s.t, s.ctx, time.Minute, "custom quota observed by the real admission controller", func() bool {
		q := &corev1.ResourceQuota{}
		if err := w.Get(s.ctx, client.ObjectKey{Namespace: "hf-team-a", Name: bootstrap.QuotaName}, q); err != nil {
			return false
		}
		actual := q.Status.Hard[corev1.ResourceRequestsCPU]
		return actual.String() == "2"
	})
	// Sync the declared profile through the same authenticated API as the admin UI.
	s.api("admin", "POST", "/api/v1/tenants", uuid.NewString(), map[string]string{"slug": "team-a", "name": "team-a", "cluster_id": s.A, "owner_id": s.sessions["owner-a"].User.ID}, 201, nil)
	key := client.ObjectKey{Namespace: "hf-team-a", Name: bootstrap.ReceiptName}
	before := &corev1.ConfigMap{}
	if err := w.Get(s.ctx, key, before); err != nil {
		s.t.Fatal(err)
	}
	run("bash", filepath.Join(root, "stages", s.stage, "run.sh"), "hub-recreate")
	quota := &corev1.ResourceQuota{}
	if err := w.Get(s.ctx, client.ObjectKey{Namespace: "hf-team-a", Name: bootstrap.QuotaName}, quota); err != nil {
		s.t.Fatal(err)
	}
	q := quota.Spec.Hard[corev1.ResourceRequestsCPU]
	if q.String() != "2" {
		s.t.Fatalf("Hub recreate reset custom requests.cpu: %s", q.String())
	}
	after := &corev1.ConfigMap{}
	if err := w.Get(s.ctx, key, after); err != nil || before.UID != after.UID || !reflect.DeepEqual(before.Data, after.Data) {
		s.t.Fatal("Hub recreate changed trusted tenant receipt")
	}
	var profiles []domain.TenantProfile
	s.api("admin", "GET", "/api/v1/tenants/team-a/quota", "", nil, 200, &profiles)
	found := false
	for _, p := range profiles {
		if p.ClusterID == s.A && p.Hard["requests.cpu"] == "2" {
			found = true
		}
	}
	if !found {
		s.t.Fatal("Hub DB profile no longer matches custom Worker quota")
	}
	if s.stage == "m5" {
		raw, err := os.ReadFile(filepath.Join(state, s.A+"-values.json"))
		var values struct {
			Rollout      int `json:"rollout"`
			Heartbeat    int `json:"heartbeatSeconds"`
			ReconnectMin int `json:"reconnectMinSeconds"`
			ReconnectMax int `json:"reconnectMaxSeconds"`
		}
		if err != nil || json.Unmarshal(raw, &values) != nil || values.Rollout != 3 || values.Heartbeat != 4 || values.ReconnectMin != 2 || values.ReconnectMax != 40 {
			s.t.Fatal("Hub recreate reset persisted Helm rollout or custom timing")
		}
		d := &appsv1.Deployment{}
		if err := w.Get(s.ctx, client.ObjectKey{Namespace: bootstrap.ControlNamespace, Name: "hugofleet-agent"}, d); err != nil {
			s.t.Fatal(err)
		}
		if d.Spec.Template.Annotations["platform.hugofleet.io/rollout"] != "3" || len(d.Spec.Template.Spec.Containers) != 1 {
			s.t.Fatal("Hub recreate reset live Agent rollout")
		}
		for _, wanted := range []string{"--heartbeat=4s", "--reconnect-min=2s", "--reconnect-max=40s"} {
			found := false
			for _, actual := range d.Spec.Template.Spec.Containers[0].Args {
				if actual == wanted {
					found = true
				}
			}
			if !found {
				s.t.Fatalf("Hub recreate lost live Agent timing %s", wanted)
			}
		}
	}
	s.t.Log("PASS real Hub recreate retains custom Worker quota, receipt UID/data, Hub profile, session and Helm rollout while reconnecting Agents")
}
