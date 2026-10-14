//go:build helmintegration

package chart

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func project(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func render(t *testing.T, root, release, namespace string, overrides map[string]any) ([]byte, error) {
	t.Helper()
	values := map[string]any{"cluster": "hugofleet-m5-e2e-a", "hubEndpoint": "172.18.0.2:18443", "tenants": []string{"team-a", "team-b"}, "rollout": 1}
	for k, v := range overrides {
		values[k] = v
	}
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "values.json")
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, filepath.Join(root, ".tools/bin/helm"), "template", release, filepath.Join(root, "stages/m5/chart"), "--namespace", namespace, "--include-crds", "--values", file).CombinedOutput()
}
func TestActualHelmRenderSecurityAndPreservedMaterial(t *testing.T) {
	root := project(t)
	raw, err := render(t, root, "hugofleet-worker", "hf-system", nil)
	if err != nil {
		t.Fatalf("Helm failed: %v %s", err, raw)
	}
	source, err := os.ReadFile(filepath.Join(root, "stages/m1/config/crd/bases/platform.hugofleet.io_applications.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	packaged, err := os.ReadFile(filepath.Join(root, "stages/m5/chart/crds/platform.hugofleet.io_applications.yaml"))
	if err != nil || !bytes.Equal(source, packaged) {
		t.Fatal("Chart CRD differs from the generated authoritative CRD")
	}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(raw), 4096)
	count, deployments, roles := 0, 0, 0
	for {
		var doc json.RawMessage
		err = decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(doc) == 0 || string(doc) == "null" {
			continue
		}
		var identity struct {
			Kind     string
			Metadata struct {
				Name, Namespace     string
				Labels, Annotations map[string]string
			}
		}
		if err = json.Unmarshal(doc, &identity); err != nil {
			t.Fatal(err)
		}
		count++
		if identity.Kind != "CustomResourceDefinition" {
			if identity.Metadata.Namespace != "hf-team-a" && identity.Metadata.Namespace != "hf-team-b" && identity.Metadata.Namespace != "hf-system" {
				t.Fatal("object escaped prepared namespaces")
			}
			if identity.Metadata.Labels["app.kubernetes.io/managed-by"] != "Helm" || identity.Metadata.Labels["platform.hugofleet.io/cluster"] != "hugofleet-m5-e2e-a" || identity.Metadata.Annotations["meta.helm.sh/release-name"] != "hugofleet-worker" || identity.Metadata.Annotations["meta.helm.sh/release-namespace"] != "hf-system" {
				t.Fatal("object has no exact release ownership")
			}
		}
		switch identity.Kind {
		case "CustomResourceDefinition":
			if identity.Metadata.Name != "applications.platform.hugofleet.io" {
				t.Fatal("unexpected CRD")
			}
		case "Role":
			roles++
			var role rbacv1.Role
			if err = json.Unmarshal(doc, &role); err != nil {
				t.Fatal(err)
			}
			for _, rule := range role.Rules {
				for _, v := range append(append([]string{}, rule.Verbs...), rule.Resources...) {
					if v == "*" || v == "secrets" || v == "pods/exec" || v == "namespaces" {
						t.Fatal("chart introduced privileged RBAC")
					}
				}
				if len(rule.NonResourceURLs) != 0 {
					t.Fatal("unexpected non-resource permission")
				}
				for _, r := range rule.Resources {
					if r == "configmaps" && len(rule.ResourceNames) != 1 {
						t.Fatal("ledger/profile access is not restricted to a named ConfigMap")
					}
				}
			}
		case "RoleBinding":
			var binding rbacv1.RoleBinding
			if err = json.Unmarshal(doc, &binding); err != nil {
				t.Fatal(err)
			}
			if binding.RoleRef.Kind != "Role" || len(binding.Subjects) != 1 || binding.Subjects[0].Kind != "ServiceAccount" {
				t.Fatal("non-namespaced or arbitrary subject RBAC")
			}
		case "ServiceAccount":
		case "Deployment":
			deployments++
			var d appsv1.Deployment
			if err = json.Unmarshal(doc, &d); err != nil {
				t.Fatal(err)
			}
			pod := d.Spec.Template.Spec
			if d.Spec.Replicas == nil || *d.Spec.Replicas != 1 || d.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType || pod.HostNetwork || pod.HostPID || pod.HostIPC || len(pod.InitContainers) != 0 || len(pod.Containers) != 1 || pod.SecurityContext == nil || pod.SecurityContext.RunAsNonRoot == nil || !*pod.SecurityContext.RunAsNonRoot || pod.SecurityContext.RunAsUser == nil || *pod.SecurityContext.RunAsUser != 65532 || pod.SecurityContext.SeccompProfile == nil || pod.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
				t.Fatal("unsafe or unbounded Pod")
			}
			c := pod.Containers[0]
			sc := c.SecurityContext
			if sc == nil || sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation || sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem || sc.Capabilities == nil || !reflect.DeepEqual(sc.Capabilities.Drop, []corev1.Capability{"ALL"}) || len(c.Command) != 0 || len(c.Env) != 0 {
				t.Fatal("unsafe container security or arbitrary command/environment")
			}
			if c.Resources.Requests.Cpu().Sign() <= 0 || c.Resources.Requests.Memory().Sign() <= 0 || c.Resources.Limits.Cpu().Sign() <= 0 || c.Resources.Limits.Memory().Sign() <= 0 {
				t.Fatal("unbounded compute resources")
			}
			if identity.Metadata.Namespace == "hf-system" {
				for _, arg := range []string{"--heartbeat=5s", "--reconnect-min=1s", "--reconnect-max=30s"} {
					if !containsArg(c.Args, arg) {
						t.Fatalf("Agent missing default timing flag %s", arg)
					}
				}
				if c.Image != "hugofleet-agent:m5" || len(pod.Volumes) != 1 || pod.Volumes[0].Secret == nil || pod.Volumes[0].Secret.SecretName != "hugofleet-agent-tls" || len(c.VolumeMounts) != 1 || !c.VolumeMounts[0].ReadOnly {
					t.Fatal("Agent does not reference the existing Worker-only TLS secret")
				}
			} else {
				if c.Image != "hugofleet-operator:m1" || len(pod.Volumes) != 0 || c.ReadinessProbe == nil || c.LivenessProbe == nil {
					t.Fatal("Operator image/probes/volumes invalid")
				}
			}
		default:
			t.Fatalf("Chart owns material that must survive uninstall: %s", identity.Kind)
		}
	}
	if count != 17 || deployments != 3 || roles != 5 {
		t.Fatalf("chart objects=%d deployments=%d roles=%d", count, deployments, roles)
	}
}
func TestActualHelmRejectsUntrustedScopeAndArbitraryValues(t *testing.T) {
	root := project(t)
	for _, v := range []struct {
		name, release, namespace string
		values                   map[string]any
	}{
		{"foreign-release", "foreign", "hf-system", nil}, {"foreign-namespace", "hugofleet-worker", "default", nil},
		{"foreign-cluster", "hugofleet-worker", "hf-system", map[string]any{"cluster": "foreign"}},
		{"public-endpoint", "hugofleet-worker", "hf-system", map[string]any{"hubEndpoint": "203.0.113.2:18443"}},
		{"duplicate-tenants", "hugofleet-worker", "hf-system", map[string]any{"tenants": []string{"team-a", "team-a"}}},
		{"foreign-secret", "hugofleet-worker", "hf-system", map[string]any{"tlsSecret": "foreign"}},
		{"arbitrary-shell", "hugofleet-worker", "hf-system", map[string]any{"command": []string{"sh"}}},
		{"secret-value", "hugofleet-worker", "hf-system", map[string]any{"clientSecret": "test-value"}},
		{"invalid-rollout", "hugofleet-worker", "hf-system", map[string]any{"rollout": 0}},
		{"heartbeat-zero", "hugofleet-worker", "hf-system", map[string]any{"heartbeatSeconds": 0}},
		{"heartbeat-too-large", "hugofleet-worker", "hf-system", map[string]any{"heartbeatSeconds": 31}},
		{"heartbeat-fractional", "hugofleet-worker", "hf-system", map[string]any{"heartbeatSeconds": 1.5}},
		{"heartbeat-string", "hugofleet-worker", "hf-system", map[string]any{"heartbeatSeconds": "5"}},
		{"reconnect-min-zero", "hugofleet-worker", "hf-system", map[string]any{"reconnectMinSeconds": 0}},
		{"reconnect-min-too-large", "hugofleet-worker", "hf-system", map[string]any{"reconnectMinSeconds": 301}},
		{"reconnect-max-zero", "hugofleet-worker", "hf-system", map[string]any{"reconnectMaxSeconds": 0}},
		{"reconnect-max-too-large", "hugofleet-worker", "hf-system", map[string]any{"reconnectMaxSeconds": 301}},
		{"reconnect-reversed", "hugofleet-worker", "hf-system", map[string]any{"reconnectMinSeconds": 31, "reconnectMaxSeconds": 30}},
	} {
		t.Run(v.name, func(t *testing.T) {
			if _, err := render(t, root, v.release, v.namespace, v.values); err == nil {
				t.Fatal("Helm accepted untrusted scope/values")
			}
		})
	}
}

func containsArg(args []string, wanted string) bool {
	for _, arg := range args {
		if arg == wanted {
			return true
		}
	}
	return false
}

func TestActualHelmTimingOverridesReachAgent(t *testing.T) {
	for _, timing := range []struct{ heartbeat, min, max int }{{7, 2, 40}, {1, 1, 1}, {30, 300, 300}} {
		raw, err := render(t, project(t), "hugofleet-worker", "hf-system", map[string]any{"heartbeatSeconds": timing.heartbeat, "reconnectMinSeconds": timing.min, "reconnectMaxSeconds": timing.max})
		if err != nil {
			t.Fatalf("valid timing rejected: %v %s", err, raw)
		}
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(raw), 4096)
		found := false
		for {
			var d appsv1.Deployment
			if err := decoder.Decode(&d); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			if d.Kind != "Deployment" || d.Namespace != "hf-system" {
				continue
			}
			found = true
			if len(d.Spec.Template.Spec.Containers) != 1 {
				t.Fatal("expected exactly one Agent container")
			}
			args := d.Spec.Template.Spec.Containers[0].Args
			for _, expected := range []string{fmt.Sprintf("--heartbeat=%ds", timing.heartbeat), fmt.Sprintf("--reconnect-min=%ds", timing.min), fmt.Sprintf("--reconnect-max=%ds", timing.max)} {
				if !containsArg(args, expected) {
					t.Fatalf("configured timing did not reach Agent: %s", expected)
				}
			}
		}
		if !found {
			t.Fatal("no rendered Agent Deployment")
		}
	}
}

func TestChartContainsNoPrivateCredentialMaterial(t *testing.T) {
	root := project(t)
	if err := filepath.WalkDir(filepath.Join(root, "stages/m5/chart"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		for _, v := range []string{"BEGIN PRIVATE KEY", "BEGIN EC PRIVATE KEY", "BEGIN RSA PRIVATE KEY", "client_secret:"} {
			if strings.Contains(string(raw), v) {
				t.Fatal("private material in chart")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
