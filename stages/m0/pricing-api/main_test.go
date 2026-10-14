package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type startupWriter struct{ started chan struct{} }

func (w *startupWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "server_started") {
		close(w.started)
	}
	return len(data), nil
}

func TestRunGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &startupWriter{started: make(chan struct{})}
	logger := slog.New(slog.NewJSONHandler(writer, nil))
	result := make(chan error, 1)
	go func() { result <- run(ctx, "127.0.0.1:0", logger) }()
	select {
	case <-writer.started:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("graceful shutdown failed: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestRunReportsListenFailure(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if err := run(context.Background(), "invalid-address", logger); err == nil {
		t.Fatal("invalid listen address accepted")
	}
}
