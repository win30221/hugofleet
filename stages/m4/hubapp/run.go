package hubapp

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m2/hub/service"
	"github.com/win30221/hugofleet/stages/m3/agentprotocol"
	"github.com/win30221/hugofleet/stages/m3/agenttls"
	"github.com/win30221/hugofleet/stages/m3/agenttransport"
	"github.com/win30221/hugofleet/stages/m4/auth"
	"github.com/win30221/hugofleet/stages/m4/httpapi"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type config struct {
	OIDC         auth.Config `json:"oidc"`
	AdminSubject string      `json:"admin_subject"`
	Clusters     []string    `json:"clusters"`
}

type Options struct {
	API          httpapi.Options
	Telemetry    bool
	Mode, Listen string
}

func Run(options Options) error {
	if options.Mode == "" {
		options.Mode = "m4"
	}
	if options.Listen == "" {
		options.Listen = "127.0.0.1:18094"
	}
	f := flag.NewFlagSet("hugofleet-oidc-hub", flag.ContinueOnError)
	addr := f.String("listen", options.Listen, "HTTP loopback Demo or explicit HTTPS listener")
	grpcAddr := f.String("grpc-listen", ":18443", "mTLS Worker control listener")
	path := f.String("database", "", "private SQLite security database")
	configPath := f.String("config", "", "private trusted OIDC/cluster configuration JSON")
	certs := f.String("certs", "", "Hub-only mTLS credentials directory; never Worker kubeconfig")
	tlsCert := f.String("https-cert", "", "HTTPS server certificate for normal deployment")
	tlsKey := f.String("https-key", "", "HTTPS server private key for normal deployment")
	ttl := f.Duration("session-ttl", time.Hour, "platform session TTL, bounded by ID Token expiry")
	timeout := f.Duration("operation-timeout", 180*time.Second, "persisted operation deadline")
	interval := f.Duration("poll-interval", time.Second, "bounded reconciliation interval")
	heartbeatTimeout := f.Duration("heartbeat-timeout", 15*time.Second, "Agent expiry threshold (3s..5m)")
	if err := f.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if err := agentprotocol.ValidateHeartbeatTimeout(*heartbeatTimeout); err != nil {
		return err
	}
	if f.NArg() != 0 || *path == "" || *configPath == "" || *certs == "" || *timeout < time.Second || *timeout > 30*time.Minute || *interval < 100*time.Millisecond || *interval > time.Minute {
		return fmt.Errorf("explicit DB/config/certificates and bounded durations required")
	}
	if os.Getenv("DEV_AUTH_MODE") != "" {
		return fmt.Errorf("M4 never enables DEV_AUTH_MODE; verified OIDC is mandatory")
	}
	file, err := os.Open(*configPath)
	if err != nil {
		return fmt.Errorf("trusted OIDC config missing")
	}
	var c config
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	decodeErr := decoder.Decode(&c)
	var extra any
	tailErr := decoder.Decode(&extra)
	closeErr := file.Close()
	if decodeErr != nil || tailErr != io.EOF || closeErr != nil {
		return fmt.Errorf("invalid trusted OIDC configuration")
	}
	c.OIDC.SessionTTL = *ttl
	if err = c.OIDC.Validate(); err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(*addr)
	if err != nil || port == "0" {
		return fmt.Errorf("explicit listener port required")
	}
	if c.OIDC.LocalDemo {
		if (c.OIDC.HTTPClientIP == "" && host != "127.0.0.1") || (c.OIDC.HTTPClientIP != "" && host != "0.0.0.0") || *tlsCert != "" || *tlsKey != "" {
			return fmt.Errorf("HTTP Demo requires loopback binding or exact Docker gateway plus loopback publication")
		}
	} else if *tlsCert == "" || *tlsKey == "" {
		return fmt.Errorf("normal OIDC deployment requires HTTPS certificates")
	}
	if len(c.Clusters) != 2 || c.Clusters[0] == c.Clusters[1] {
		return fmt.Errorf("exact two distinct Worker registry identities required")
	}
	for _, id := range c.Clusters {
		switch id {
		case "hugofleet-a", "hugofleet-b", "hugofleet-m4-e2e-a", "hugofleet-m4-e2e-b":
		case "hugofleet-m5-e2e-a", "hugofleet-m5-e2e-b":
			if options.Mode != "m5" {
				return fmt.Errorf("M5 Worker identity requires M5 entrypoint")
			}
		default:
			return fmt.Errorf("unsupported project Worker identity")
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var telemetry *observability.Runtime
	if options.Telemetry {
		telemetry, err = observability.New("hub", os.Stdout, c.Clusters)
		if err != nil {
			return err
		}
		defer func() {
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if e := telemetry.Provider.Shutdown(shutdown); e != nil {
				slog.Warn("telemetry shutdown incomplete")
			}
		}()
		options.API.Metrics = telemetry.Metrics.Handler()
		options.API.Traces = telemetry.Exporter.Records
	}
	startCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	store, err := repository.Open(startCtx, *path)
	if err != nil {
		return err
	}
	defer store.Close()
	if telemetry != nil {
		store.Terminal = func(o domain.Operation, a domain.Application) {
			telemetry.Metrics.Terminal(o.Type, o.State, o.CreatedAt, o.FinishedAt)
			slog.Info("Operation completed", "request_id", o.RequestID, "operation_id", o.ID, "cluster_id", a.ClusterID, "tenant_id", a.TenantID, "app_id", a.ID, "state", o.State, "error_code", o.ErrorCode)
		}
	}
	if err = store.ConfigureSecurity(startCtx, c.Clusters, c.OIDC.Issuer, c.AdminSubject); err != nil {
		return err
	}
	oidcAuth, err := auth.New(startCtx, c.OIDC, store)
	if err != nil {
		return err
	}
	cfg, err := agenttls.Config(*certs, "hub", true)
	if err != nil {
		return err
	}
	remote := agenttransport.New(c.Clusters)
	remote.HeartbeatTimeout = *heartbeatTimeout
	rpc := remote.Server(cfg)
	rpcListener, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		return err
	}
	defer rpcListener.Close()
	svc := &service.Service{Store: store, Policy: domain.DefaultPolicy(), Timeout: *timeout}
	api, err := httpapi.NewWithOptions(oidcAuth, svc, remote, options.API)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	runner := &service.Runner{Store: store, Transport: remote, Interval: *interval}
	if telemetry != nil {
		runner.Connection = telemetry.Metrics.Connection
		runner.ReconcileError = telemetry.Metrics.ReconcileError
	}
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	serve := make(chan error, 2)
	go func() { serve <- rpc.Serve(rpcListener) }()
	go func() {
		if c.OIDC.LocalDemo {
			serve <- server.Serve(listener)
		} else {
			serve <- server.ServeTLS(listener, *tlsCert, *tlsKey)
		}
	}()
	slog.Info("OIDC Hub started", "url", c.OIDC.BaseURL, "mode", options.Mode, "cluster_count", len(c.Clusters))
	select {
	case <-ctx.Done():
	case err = <-serve:
		stop()
	}
	shutdown, finish := context.WithTimeout(context.Background(), 10*time.Second)
	defer finish()
	shutdownErr := server.Shutdown(shutdown)
	rpc.Stop()
	<-done
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, shutdownErr)
}
