package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/httpapi"
	"github.com/win30221/hugofleet/stages/m2/hub/kube"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m2/hub/service"
	"github.com/win30221/hugofleet/stages/m3/agentprotocol"
	"github.com/win30221/hugofleet/stages/m3/agenttls"
	"github.com/win30221/hugofleet/stages/m3/agenttransport"
	"google.golang.org/grpc"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("Hub stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	// controller-runtime registers kubeconfig on the process-wide FlagSet.
	// Own our CLI namespace so the two registrations cannot collide.
	flags := flag.NewFlagSet("hugofleet-hub", flag.ContinueOnError)
	var addr, path, kubeconfig, contextName, policyPath string
	var timeout, interval time.Duration
	var mode, grpcAddr, certs, tokenPath, clusterIDs, allowedIP, publicHost string
	flags.StringVar(&mode, "mode", "m2", "m2 local development or m3 outbound agents")
	flags.StringVar(&grpcAddr, "grpc-listen", ":18443", "M3 mTLS control listener")
	flags.StringVar(&certs, "certs", "", "M3 Hub-only credential directory")
	flags.StringVar(&tokenPath, "admin-token-file", "", "M3 private Demo token file")
	flags.StringVar(&clusterIDs, "clusters", "hugofleet-a,hugofleet-b", "M3 registered cluster IDs")
	flags.StringVar(&allowedIP, "http-client-ip", "", "M3 exact Docker gateway allowed for HTTP management")
	flags.StringVar(&publicHost, "http-host", "127.0.0.1:18090", "M3 expected local published Host")
	flags.StringVar(&addr, "listen", "127.0.0.1:18090", "loopback HTTP address; DEV_AUTH_MODE only")
	flags.StringVar(&path, "database", ".local/hub/hub.db", "SQLite file on a local filesystem")
	flags.StringVar(&kubeconfig, "kubeconfig", "", "restricted, bootstrap-generated development kubeconfig")
	flags.StringVar(&contextName, "context", "hugofleet-hub", "restricted kubeconfig context")
	flags.StringVar(&policyPath, "policy", "stages/m2/policy.json", "resource defaults and maxima")
	flags.DurationVar(&timeout, "operation-timeout", 180*time.Second, "persisted operation deadline")
	flags.DurationVar(&interval, "poll-interval", time.Second, "bounded reconciliation interval")
	heartbeatTimeout := flags.Duration("heartbeat-timeout", 15*time.Second, "M3 Agent expiry threshold (3s..5m)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if err := agentprotocol.ValidateHeartbeatTimeout(*heartbeatTimeout); err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if mode == "m2" && (host != "127.0.0.1" || port == "0") {
		return fmt.Errorf("M2 requires an explicit 127.0.0.1 loopback port")
	}
	if (mode == "m2" && kubeconfig == "") || timeout < time.Second || timeout > 30*time.Minute || interval < 100*time.Millisecond || interval > time.Minute {
		return fmt.Errorf("kubeconfig and valid bounded durations are required")
	}
	if mode != "m2" && mode != "m3" {
		return fmt.Errorf("unsupported Hub mode")
	}
	if mode == "m3" && (kubeconfig != "" || certs == "" || tokenPath == "" || net.ParseIP(allowedIP) == nil || host != "0.0.0.0" || port == "0") {
		return fmt.Errorf("M3 requires container listener, verified certificates, token, gateway restriction, and no Worker kubeconfig")
	}
	policy, err := loadPolicy(policyPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	openCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	store, err := repository.Open(openCtx, path)
	cancel()
	if err != nil {
		return err
	}
	defer store.Close()
	var transport service.Transport
	var rpc *grpc.Server
	var rpcListener net.Listener
	var token string
	if mode == "m2" {
		if err = store.CheckLocal(ctx); err != nil {
			return err
		}
		transport, err = kube.New(kubeconfig, contextName)
	} else {
		ids := strings.Split(clusterIDs, ",")
		if len(ids) != 2 || ids[0] == ids[1] {
			return fmt.Errorf("M3 requires two distinct registered clusters")
		}
		for _, id := range ids {
			if id != "hugofleet-a" && id != "hugofleet-b" && id != "hugofleet-m3-e2e-a" && id != "hugofleet-m3-e2e-b" {
				return fmt.Errorf("unsupported project cluster identity")
			}
		}
		if err = store.ConfigureRemote(ctx, ids); err != nil {
			return err
		}
		var raw []byte
		raw, err = os.ReadFile(tokenPath)
		if err != nil {
			return err
		}
		token = strings.TrimSpace(string(raw))
		if len(token) != 64 {
			return fmt.Errorf("M3 requires a random 32-byte hexadecimal management token")
		}
		cfg, e := agenttls.Config(certs, "hub", true)
		if e != nil {
			return e
		}
		remote := agenttransport.New(ids)
		remote.HeartbeatTimeout = *heartbeatTimeout
		transport = remote
		rpc = remote.Server(cfg)
		rpcListener, err = net.Listen("tcp", grpcAddr)
		publicIP, _, e := net.SplitHostPort(publicHost)
		if e != nil || publicIP != "127.0.0.1" {
			return fmt.Errorf("M3 HTTP Host must be the published loopback endpoint")
		}
	}
	if err != nil {
		return err
	}
	svc := &service.Service{Store: store, Policy: policy, Timeout: timeout}
	apiHost := addr
	if mode == "m3" {
		apiHost = publicHost
	}
	api, err := httpapi.New(svc, transport, apiHost)
	if err != nil {
		return err
	}
	api.Token = token
	api.AllowedHTTPIP = allowedIP
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	runner := &service.Runner{Store: store, Transport: transport, Interval: interval}
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	serve := make(chan error, 1)
	go func() { serve <- server.Serve(listener) }()
	if rpc != nil {
		go func() {
			if e := rpc.Serve(rpcListener); e != nil {
				slog.Error("gRPC server stopped", "error", e)
				stop()
			}
		}()
	}
	slog.Info("development Hub started", "mode", mode, "url", "http://"+apiHost, "tenant", domain.TenantID)
	select {
	case <-ctx.Done():
	case err = <-serve:
		stop()
	}
	shutdown, closeCtx := context.WithTimeout(context.Background(), 10*time.Second)
	defer closeCtx()
	shutdownErr := server.Shutdown(shutdown)
	if rpc != nil {
		rpc.Stop()
		rpcListener.Close()
	}
	<-done
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, shutdownErr)
}
