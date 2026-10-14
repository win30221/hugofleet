package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Metrics inside a Pod are loopback-only; access is an explicit local diagnostic
// port-forward, never an unauthenticated management listener on the Pod network.
func ServeLoopback(ctx context.Context, address string, handler http.Handler) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" || port == "" || port == "0" {
		return fmt.Errorf("metrics require an explicit 127.0.0.1 listener")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", handler)
	s := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8 << 10}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		s.Shutdown(shutdown)
	}()
	go func() {
		if err := s.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("loopback metrics server stopped")
		}
	}()
	return nil
}

type Correlation struct{ RequestID, OperationID, ClusterID, TenantID, AppID string }

func SafeCorrelation(v Correlation) Correlation {
	if !uuidPattern.MatchString(v.RequestID) {
		v.RequestID = ""
	}
	if !uuidPattern.MatchString(v.OperationID) {
		v.OperationID = ""
	}
	if !uuidPattern.MatchString(v.AppID) {
		v.AppID = ""
	}
	if !slugPattern.MatchString(v.ClusterID) {
		v.ClusterID = ""
	}
	if !slugPattern.MatchString(v.TenantID) {
		v.TenantID = ""
	}
	return v
}
func LogAttributes(v Correlation) []any {
	v = SafeCorrelation(v)
	a := []any{}
	for _, p := range [][2]string{{"request_id", v.RequestID}, {"operation_id", v.OperationID}, {"cluster_id", v.ClusterID}, {"tenant_id", v.TenantID}, {"app_id", v.AppID}} {
		if p[1] != "" {
			a = append(a, p[0], p[1])
		}
	}
	return a
}

type correlationKey struct{}

type channelKey struct{}

func WithChannel(ctx context.Context, id string) context.Context {
	if !uuidPattern.MatchString(id) {
		return ctx
	}
	return context.WithValue(ctx, channelKey{}, id)
}
func ChannelContext(ctx context.Context) string { v, _ := ctx.Value(channelKey{}).(string); return v }

func WithCorrelation(ctx context.Context, v Correlation) context.Context {
	return context.WithValue(ctx, correlationKey{}, v)
}
func CorrelationContext(ctx context.Context) Correlation {
	v, _ := ctx.Value(correlationKey{}).(Correlation)
	return v
}

// Typed CR annotations permit Operator log correlation without arbitrary Pod
// metadata, environment, executable commands or an additional API permission.
func Annotations(ctx context.Context) map[string]string {
	parent, state := Inject(ctx)
	if parent == "" {
		return nil
	}
	c := CorrelationContext(ctx)
	v := map[string]string{"platform.hugofleet.io/traceparent": parent}
	if state != "" {
		v["platform.hugofleet.io/tracestate"] = state
	}
	for _, p := range [][2]string{{"request-id", c.RequestID}, {"operation-id", c.OperationID}} {
		if uuidPattern.MatchString(p[1]) {
			v["platform.hugofleet.io/"+p[0]] = p[1]
		}
	}
	if slugPattern.MatchString(c.ClusterID) {
		v["platform.hugofleet.io/cluster-id"] = c.ClusterID
	}
	return v
}
