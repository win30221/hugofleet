package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m1/controller"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	telemetryEnabled := flag.Bool("telemetry", false, "enable M5 JSON spans and loopback metrics")
	namespace := flag.String("namespace", "hf-demo", "Single bootstrap namespace to reconcile")
	probeAddress := flag.String("health-probe-bind-address", "127.0.0.1:8081", "Health probe address")
	options := zap.Options{Development: false}
	options.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&options)))
	log := ctrl.Log.WithName("operator")
	ctx := ctrl.SetupSignalHandler()
	var telemetry *observability.Runtime
	if *telemetryEnabled {
		var err error
		telemetry, err = observability.New("operator", os.Stdout, nil)
		if err != nil {
			log.Error(err, "telemetry setup failed")
			os.Exit(1)
		}
		defer func() {
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			telemetry.Provider.Shutdown(shutdown)
		}()
		if err = observability.ServeLoopback(ctx, "127.0.0.1:8082", telemetry.Metrics.Handler()); err != nil {
			log.Error(err, "loopback metrics setup failed")
			os.Exit(1)
		}
	}
	if *namespace == "" {
		log.Error(nil, "namespace must be explicit")
		os.Exit(1)
	}
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		log.Error(err, "core scheme failed")
		os.Exit(1)
	}
	if err := platform.AddToScheme(scheme); err != nil {
		log.Error(err, "application scheme failed")
		os.Exit(1)
	}
	config := ctrl.GetConfigOrDie()
	config.Timeout = 15 * time.Second
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme, Cache: cache.Options{DefaultNamespaces: map[string]cache.Config{*namespace: {}}},
		Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: *probeAddress,
		LeaderElection: false, GracefulShutdownTimeout: ptr(20 * time.Second),
	})
	if err != nil {
		log.Error(err, "manager setup failed")
		os.Exit(1)
	}
	reconciler := &controller.ApplicationReconciler{Client: mgr.GetClient(), Scheme: scheme}
	if telemetry != nil {
		reconciler.ReconcileError = telemetry.Metrics.ReconcileError
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		log.Error(err, "controller setup failed")
		os.Exit(1)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Error(err, "health check setup failed")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("cache", func(req *http.Request) error {
		ctx, cancel := context.WithTimeout(req.Context(), time.Second)
		defer cancel()
		if mgr.GetCache().WaitForCacheSync(ctx) {
			return nil
		}
		return fmt.Errorf("cache not synced")
	}); err != nil {
		log.Error(err, "ready check setup failed")
		os.Exit(1)
	}
	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "manager stopped with error")
		os.Exit(1)
	}
}

func ptr[T any](v T) *T { return &v }
