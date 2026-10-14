//go:build integration

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func TestEnvtest(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.WriteTo(os.Stdout)))
	env := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")}, ErrorIfCRDPathMissing: true, ControlPlaneStartTimeout: 2 * time.Minute, ControlPlaneStopTimeout: time.Minute}
	// The isolated race container has loopback only and no default network route.
	env.ControlPlane.GetAPIServer().Configure().Set("advertise-address", "127.0.0.1")
	// envtest has no service dataplane; the default lease reconciler rejects loopback.
	env.ControlPlane.GetAPIServer().Configure().Set("endpoint-reconciler-type", "none")
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("start real envtest: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var managerDone chan error
	t.Cleanup(func() {
		cancel()
		if managerDone != nil {
			select {
			case err := <-managerDone:
				if err != nil {
					t.Errorf("manager shutdown: %v", err)
				}
			case <-time.After(30 * time.Second):
				t.Error("manager shutdown timed out")
			}
		}
		if err := env.Stop(); err != nil {
			t.Errorf("stop envtest: %v", err)
		}
	})
	cfg.Timeout = 10 * time.Second
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := platform.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hf-demo"}}); err != nil {
		t.Fatal(err)
	}
	newApp := func(name string) *platform.Application {
		a := testApp()
		a.Name = name
		a.UID = ""
		a.Generation = 0
		a.Spec.Revision = 1
		a.Spec.Replicas = 1
		a.Spec.Image = "pricing-api:v1"
		a.Status = platform.ApplicationStatus{}
		return a
	}
	t.Run("schema defaults and status subresource", func(t *testing.T) {
		a := newApp("defaults")
		a.Spec.Replicas = 0
		a.Spec.Port = 0
		a.Spec.ReadinessPath = ""
		a.Spec.LivenessPath = ""
		a.Status.ObservedRevision = 99
		if err := c.Create(ctx, a); err != nil {
			t.Fatal(err)
		}
		if a.Spec.Replicas != 1 || a.Spec.Port != 8080 || a.Spec.ReadinessPath != "/readyz" || a.Spec.LivenessPath != "/healthz" || a.Status.ObservedRevision != 0 {
			t.Fatalf("defaults/subresource incorrect: %+v", a)
		}
	})
	t.Run("schema rejects invalid specs", func(t *testing.T) {
		cases := []func(*platform.Application){
			func(a *platform.Application) { a.Name = strings.Repeat("a", 64) },
			func(a *platform.Application) { a.Spec.Revision = 0 }, func(a *platform.Application) { a.Spec.Image = "" }, func(a *platform.Application) { a.Spec.Image = "bad image" }, func(a *platform.Application) { a.Spec.Image = "https://example.com/app" },
			func(a *platform.Application) { a.Spec.Replicas = -1 }, func(a *platform.Application) { a.Spec.Replicas = 4 }, func(a *platform.Application) { a.Spec.Port = 65536 },
			func(a *platform.Application) { a.Spec.Resources.Requests.CPU = "not-a-quantity" }, func(a *platform.Application) { a.Spec.Resources.Limits.Memory = "32Mi" }, func(a *platform.Application) { a.Spec.Resources.Limits.CPU = "50m" },
			func(a *platform.Application) { a.Spec.Resources.Requests.Memory = "0" }, func(a *platform.Application) { a.Spec.Resources.Limits.CPU = "-1" },
			func(a *platform.Application) { a.Spec.ReadinessPath = "http://example.com/ready" }, func(a *platform.Application) { a.Spec.LivenessPath = "//host/health" },
		}
		for i, mutate := range cases {
			a := newApp(fmt.Sprintf("invalid-%d", i))
			mutate(a)
			if err := c.Create(ctx, a); !apierrors.IsInvalid(err) {
				t.Fatalf("case %d must be Invalid, got %v", i, err)
			}
		}
	})
	t.Run("workload conflicts retry without terminal status", func(t *testing.T) {
		testWorkloadConflicts(t, ctx, c, scheme, newApp)
	})
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Cache: cache.Options{DefaultNamespaces: map[string]cache.Config{"hf-demo": {}}}, Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := (&ApplicationReconciler{Client: mgr.GetClient(), Scheme: scheme}).SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	managerDone = make(chan error, 1)
	go func() { managerDone <- mgr.Start(ctx) }()
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("cache sync failed")
	}
	wait := func(t *testing.T, check func() error) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		var err error
		for time.Now().Before(deadline) {
			err = check()
			if err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("condition timeout: %v", err)
	}
	t.Run("create update repair metadata and no write loop", func(t *testing.T) {
		a := newApp("lifecycle")
		if err := c.Create(ctx, a); err != nil {
			t.Fatal(err)
		}
		key := client.ObjectKeyFromObject(a)
		var d appsv1.Deployment
		var s corev1.Service
		wait(t, func() error {
			if err := c.Get(ctx, key, &d); err != nil {
				return err
			}
			return c.Get(ctx, key, &s)
		})
		if metav1.GetControllerOf(&d).UID != a.UID || metav1.GetControllerOf(&s).UID != a.UID || s.Spec.ClusterIP == "" {
			t.Fatal("missing ownership or service allocation")
		}
		ip := s.Spec.ClusterIP
		wait(t, func() error {
			if err := c.Get(ctx, key, a); err != nil {
				return err
			}
			ready := meta.FindStatusCondition(a.Status.Conditions, "Ready")
			if ready == nil || ready.Status != metav1.ConditionFalse {
				return fmt.Errorf("envtest must not fabricate Pod readiness")
			}
			return nil
		})
		if a.Status.ObservedRevision != 0 {
			t.Fatal("no Deployment controller has observed the revision")
		}
		a.Spec.Image = "pricing-api:v2-good"
		a.Spec.Replicas = 2
		a.Spec.Revision = 2
		a.Spec.Port = 9090
		if err := c.Update(ctx, a); err != nil {
			t.Fatal(err)
		}
		wait(t, func() error {
			if err := c.Get(ctx, key, &d); err != nil {
				return err
			}
			if err := c.Get(ctx, key, &s); err != nil {
				return err
			}
			if d.Spec.Template.Spec.Containers[0].Image != a.Spec.Image || *d.Spec.Replicas != 2 || s.Spec.Ports[0].Port != 9090 {
				return fmt.Errorf("not converged")
			}
			return nil
		})
		if s.Spec.ClusterIP != ip {
			t.Fatal("allocated ClusterIP changed")
		}
		if err := c.Patch(ctx, &d, client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"annotations":{"user":"keep"}},"spec":{"replicas":1}}`))); err != nil {
			t.Fatal(err)
		}
		if err := c.Patch(ctx, &s, client.RawPatch(types.MergePatchType, []byte(`{"spec":{"selector":{"foreign":"true"}}}`))); err != nil {
			t.Fatal(err)
		}
		wait(t, func() error {
			if err := c.Get(ctx, key, &d); err != nil {
				return err
			}
			if err := c.Get(ctx, key, &s); err != nil {
				return err
			}
			if *d.Spec.Replicas != 2 || len(s.Spec.Selector) != 1 || s.Spec.Selector[UIDLabel] != string(a.UID) {
				return fmt.Errorf("drift not repaired")
			}
			return nil
		})
		if d.Annotations["user"] != "keep" {
			t.Fatal("unowned metadata overwritten")
		}
		// A real API server's defaulted fields must not cause an endless CreateOrUpdate loop.
		wait(t, func() error {
			beforeD, beforeS := d.ResourceVersion, s.ResourceVersion
			time.Sleep(500 * time.Millisecond)
			if err := c.Get(ctx, key, &d); err != nil {
				return err
			}
			if err := c.Get(ctx, key, &s); err != nil {
				return err
			}
			if d.ResourceVersion != beforeD || s.ResourceVersion != beforeS {
				return fmt.Errorf("resourceVersions still changing")
			}
			return nil
		})
	})
	t.Run("foreign same name is never adopted", func(t *testing.T) {
		for _, kind := range []string{"deployment", "service", "old-uid"} {
			t.Run(kind, func(t *testing.T) {
				a := newApp("foreign-" + kind)
				key := client.ObjectKeyFromObject(a)
				var obj client.Object
				if kind != "service" {
					d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: a.Name, Namespace: a.Namespace}}
					mutateDeployment(a, d)
					if kind == "old-uid" {
						d.OwnerReferences = []metav1.OwnerReference{{APIVersion: platform.GroupVersion.String(), Kind: "Application", Name: a.Name, UID: "previous-uid", Controller: ptr(true)}}
					}
					obj = d
				} else {
					obj = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: a.Name, Namespace: a.Namespace}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}}}
				}
				if err := c.Create(ctx, obj); err != nil {
					t.Fatal(err)
				}
				rv := obj.GetResourceVersion()
				if err := c.Create(ctx, a); err != nil {
					t.Fatal(err)
				}
				wait(t, func() error {
					if err := c.Get(ctx, key, a); err != nil {
						return err
					}
					condition := meta.FindStatusCondition(a.Status.Conditions, "Degraded")
					if condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != "OwnershipConflict" {
						return fmt.Errorf("missing conflict condition")
					}
					return nil
				})
				if err := c.Get(ctx, key, obj); err != nil {
					t.Fatal(err)
				}
				if obj.GetResourceVersion() != rv || (kind != "old-uid" && metav1.GetControllerOf(obj) != nil) {
					t.Fatal("foreign object was modified")
				}
			})
		}
	})
}
