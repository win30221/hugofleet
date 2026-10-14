package main

import (
	"context"
	"flag"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/kube"
	"github.com/win30221/hugofleet/stages/m3/agent"
	"github.com/win30221/hugofleet/stages/m3/agentprofile"
	"github.com/win30221/hugofleet/stages/m3/agentprotocol"
	"github.com/win30221/hugofleet/stages/m3/agenttls"
	"github.com/win30221/hugofleet/stages/m5/observability"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"log/slog"
	"os"
	"os/signal"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("Agent stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	f := flag.NewFlagSet("hugofleet-agent", flag.ContinueOnError)
	endpoint := f.String("hub", "", "actual Hub Docker network IP:port")
	cluster := f.String("cluster", "", "registered cluster ID")
	certs := f.String("certs", "/certs", "mounted client credentials")
	tenants := f.String("tenants", "demo", "trusted comma-separated tenant allowlist")
	ledgerNamespace := f.String("ledger-namespace", "hf-demo", "hf-demo (M3) or hf-system (M4) ledger")
	version := f.String("version", "m3", "Agent implementation stage")
	heartbeat := f.Duration("heartbeat", 5*time.Second, "heartbeat interval (1s..30s)")
	retryMin := f.Duration("reconnect-min", time.Second, "initial reconnect backoff (at least 100ms)")
	retryMax := f.Duration("reconnect-max", 30*time.Second, "maximum reconnect backoff (at most 5m)")
	if err := f.Parse(os.Args[1:]); err != nil {
		return err
	}
	if err := agentprotocol.ValidateAgentTiming(*heartbeat, *retryMin, *retryMax); err != nil {
		return err
	}
	tlsConfig, err := agenttls.Config(*certs, *cluster, false)
	if err != nil {
		return err
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	cfg.Timeout = 5 * time.Second
	if (*version != "m3" && *version != "m4" && *version != "m5") || (*version == "m3" && (*tenants != "demo" || *ledgerNamespace != domain.Namespace)) || ((*version == "m4" || *version == "m5") && *ledgerNamespace != agentprofile.ControlNamespace) {
		return domain.Problem("INVALID_ARGUMENT", "stage and ledger namespace configuration mismatch")
	}
	approved := strings.Split(*tenants, ",")
	allowed := map[string]bool{}
	for _, tenant := range approved {
		if err = domain.ValidateTenant(tenant); err != nil {
			return err
		}
		if allowed[tenant] {
			return domain.Problem("INVALID_ARGUMENT", "duplicate tenant")
		}
		allowed[tenant] = true
	}
	workloads, err := kube.NewForTenants(cfg, approved)
	if err != nil {
		return err
	}
	scheme := runtime.NewScheme()
	if err = corev1.AddToScheme(scheme); err != nil {
		return err
	}
	worker, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var connection func(bool)
	if *version == "m5" {
		telemetry, e := observability.New("agent", os.Stdout, []string{*cluster})
		if e != nil {
			return e
		}
		defer func() {
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			telemetry.Provider.Shutdown(shutdown)
		}()
		if e = observability.ServeLoopback(ctx, "127.0.0.1:8082", telemetry.Metrics.Handler()); e != nil {
			return e
		}
		connection = func(connected bool) { telemetry.Metrics.Connection(*cluster, connected) }
	}
	engine := &agent.Engine{Cluster: *cluster, Ledger: &agent.KubeLedger{Client: worker, Namespace: *ledgerNamespace}, Workloads: workloads}
	if *version == "m4" || *version == "m5" {
		engine.Tenants = allowed
		engine.Profiles = &agentprofile.Reader{Client: worker, Cluster: *cluster, Tenants: allowed}
	}
	if _, err = engine.Ledger.Load(ctx); err != nil {
		return err
	}
	return (&agent.Client{Endpoint: *endpoint, Cluster: *cluster, TLS: tlsConfig, Engine: engine, Version: *version, Heartbeat: *heartbeat, ReconnectMin: *retryMin, ReconnectMax: *retryMax, Connection: connection}).Run(ctx)
}
