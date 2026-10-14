//go:build integration

package controller

import (
	"context"
	"testing"

	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Perform a real competing API write between CreateOrUpdate's read and write.
// The API server, rather than a fake error, rejects the stale resourceVersion.
type competingWriter struct {
	client.Client
	kind     string
	injected bool
}

func (c *competingWriter) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	_, deployment := obj.(*appsv1.Deployment)
	_, service := obj.(*corev1.Service)
	if !c.injected && ((c.kind == "deployment" && deployment) || (c.kind == "service" && service)) {
		latest := obj.DeepCopyObject().(client.Object)
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), latest); err != nil {
			return err
		}
		annotations := latest.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations["test.hugofleet.io/concurrent-write"] = "preserved"
		latest.SetAnnotations(annotations)
		if err := c.Client.Update(ctx, latest); err != nil {
			return err
		}
		c.injected = true
	}
	return c.Client.Update(ctx, obj, opts...)
}

func testWorkloadConflicts(t *testing.T, ctx context.Context, c client.Client, scheme *runtime.Scheme, newApp func(string) *platform.Application) {
	for _, kind := range []string{"deployment", "service"} {
		t.Run(kind, func(t *testing.T) {
			app := newApp("conflict-" + kind)
			if err := c.Create(ctx, app); err != nil {
				t.Fatal(err)
			}
			req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(app)}
			r := &ApplicationReconciler{Client: c, Scheme: scheme}
			if _, err := r.Reconcile(ctx, req); err != nil {
				t.Fatal(err)
			}
			if err := c.Get(ctx, req.NamespacedName, app); err != nil {
				t.Fatal(err)
			}
			before := app.Status.DeepCopy()
			app.Spec.Revision++
			app.Spec.Replicas = 2
			app.Spec.Port = 9090
			if err := c.Update(ctx, app); err != nil {
				t.Fatal(err)
			}
			writer := &competingWriter{Client: c, kind: kind}
			r.Client = writer
			if _, err := r.Reconcile(ctx, req); !apierrors.IsConflict(err) || !writer.injected {
				t.Fatalf("expected real API conflict returned for retry, got %v (injected=%v)", err, writer.injected)
			}
			if err := c.Get(ctx, req.NamespacedName, app); err != nil {
				t.Fatal(err)
			}
			if !equality.Semantic.DeepEqual(*before, app.Status) {
				t.Fatalf("transient write conflict changed Application status: %+v", app.Status)
			}
			r.Client = c
			if _, err := r.Reconcile(ctx, req); err != nil {
				t.Fatal(err)
			}
			dep, svc := &appsv1.Deployment{}, &corev1.Service{}
			for _, obj := range []client.Object{dep, svc, app} {
				if err := c.Get(ctx, req.NamespacedName, obj); err != nil {
					t.Fatal(err)
				}
			}
			if *dep.Spec.Replicas != 2 || dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort != 9090 || svc.Spec.Ports[0].Port != 9090 {
				t.Fatal("retry did not converge the desired workload")
			}
			var modified client.Object = dep
			if kind == "service" {
				modified = svc
			}
			if modified.GetAnnotations()["test.hugofleet.io/concurrent-write"] != "preserved" || meta.IsStatusConditionTrue(app.Status.Conditions, "Degraded") || meta.IsStatusConditionTrue(app.Status.Conditions, "Ready") {
				t.Fatal("retry lost concurrent metadata, reported failure or fabricated readiness")
			}
		})
	}
}
