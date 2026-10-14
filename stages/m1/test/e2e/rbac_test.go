//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestM1Authorization(t *testing.T) {
	if os.Getenv("HF_CLUSTER") != "hugofleet-m1-e2e" {
		t.Fatal("only the dedicated, script-owned hugofleet-m1-e2e is permitted")
	}
	loaded, err := clientcmd.LoadFromFile(os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CurrentContext != "kind-hugofleet-m1-e2e" {
		t.Fatal("unexpected kubeconfig context")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Timeout = 10 * time.Second
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{authorizationv1.AddToScheme, rbacv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := c.Get(ctx, client.ObjectKey{Namespace: "hf-demo", Name: "hugofleet-operator"}, &rbacv1.Role{}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		verb, resource, group, namespace, subresource string
		allowed                                       bool
	}{
		{"update", "deployments", "apps", "hf-demo", "", true},
		{"get", "replicasets", "apps", "hf-demo", "", true},
		{"list", "replicasets", "apps", "hf-demo", "", true},
		{"watch", "replicasets", "apps", "hf-demo", "", true},
		{"create", "replicasets", "apps", "hf-demo", "", false},
		{"update", "replicasets", "apps", "hf-demo", "", false},
		{"get", "replicasets", "apps", "default", "", false},
		{"create", "services", "", "hf-demo", "", true},
		{"get", "secrets", "", "hf-demo", "", false},
		{"get", "pods", "", "hf-demo", "exec", false},
		{"create", "pods", "", "hf-demo", "exec", false},
		{"get", "namespaces", "", "", "", false},
		{"update", "deployments", "apps", "default", "", false},
	} {
		review := &authorizationv1.SubjectAccessReview{Spec: authorizationv1.SubjectAccessReviewSpec{
			User:               "system:serviceaccount:hf-demo:hugofleet-operator",
			Groups:             []string{"system:serviceaccounts", "system:serviceaccounts:hf-demo", "system:authenticated"},
			ResourceAttributes: &authorizationv1.ResourceAttributes{Verb: tc.verb, Resource: tc.resource, Group: tc.group, Namespace: tc.namespace, Subresource: tc.subresource},
		}}
		if err := c.Create(ctx, review); err != nil {
			t.Fatal(err)
		}
		if review.Status.EvaluationError != "" || review.Status.Allowed != tc.allowed {
			t.Fatalf("authorization %s %s/%s namespace=%s: %+v", tc.verb, tc.resource, tc.subresource, tc.namespace, review.Status)
		}
		t.Logf("PASS authorization %s %s/%s namespace=%s allowed=%t", tc.verb, tc.resource, tc.subresource, tc.namespace, tc.allowed)
	}
}
