// netprobe is an acceptance workload, not a Hub management feature. It issues
// bounded HTTP requests from an actual restricted Pod; it never invokes exec.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	serve := flag.Bool("serve", false, "serve an actual HTTP health endpoint")
	target := flag.String("target", "", "owned test HTTP Pod IP:8080/healthz")
	expect := flag.String("expect", "allowed", "allowed or blocked")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *serve {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("reachable\n")) })
		s := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 4096}
		done := make(chan error, 1)
		go func() { done <- s.ListenAndServe() }()
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return s.Shutdown(shutdown)
	}
	u, err := url.Parse(*target)
	if err != nil || u.Scheme != "http" || !net.ParseIP(u.Hostname()).IsPrivate() || u.Port() != "8080" || u.Path != "/healthz" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("explicit owned private Pod HTTP health endpoint required")
	}
	if *expect != "allowed" && *expect != "blocked" {
		return fmt.Errorf("expect must be allowed or blocked")
	}
	c := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for attempt := 0; attempt < 3; attempt++ {
		r, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if err != nil {
			return err
		}
		response, err := c.Do(r)
		if *expect == "blocked" {
			if response != nil {
				response.Body.Close()
				return fmt.Errorf("cross-tenant HTTP reached the server (HTTP %d)", response.StatusCode)
			}
			var n net.Error
			if !errors.As(err, &n) || !n.Timeout() {
				return fmt.Errorf("expected policy timeout, got a different network failure: %s", strings.ReplaceAll(fmt.Sprint(err), u.String(), "test endpoint"))
			}
		} else {
			if err != nil {
				return fmt.Errorf("positive control failed: %w", err)
			}
			response.Body.Close()
			if response.StatusCode != 200 {
				return fmt.Errorf("positive control HTTP %d", response.StatusCode)
			}
		}
	}
	fmt.Printf("PASS actual Pod HTTP probe: %s, three attempts\n", *expect)
	return nil
}
