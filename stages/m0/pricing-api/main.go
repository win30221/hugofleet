package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/win30221/hugofleet/stages/m0/pricing-api/internal/pricing"
)

// version is set by the image build; there is no runtime switch for a broken build.
var version = "v1"

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "HTTP listen address (containers use :8080)")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *listen, logger); err != nil {
		logger.Error("server_failed", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, address string, logger *slog.Logger) error {
	var ready atomic.Bool
	ready.Store(true)
	handler, err := pricing.NewHandler(pricing.Config{Version: version, Ready: ready.Load, Logger: logger})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192,
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	logger.Info("server_started", "address", listener.Addr().String(), "version", version)
	select {
	case err := <-serveResult:
		return err
	case <-ctx.Done():
		ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			closeErr := server.Close()
			return errors.Join(fmt.Errorf("shutdown: %w", err), closeErr)
		}
		if err := <-serveResult; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		logger.Info("server_stopped")
		return nil
	}
}
