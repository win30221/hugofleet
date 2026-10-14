// Package observability owns optional M5 telemetry. It imports no Hub, Agent or
// Kubernetes business packages; earlier entrypoints keep the no-op OTel default.
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var propagator = propagation.TraceContext{}

func Tracer() trace.Tracer { return otel.Tracer("hugofleet") }

// Inject and Extract intentionally exclude baggage and arbitrary HTTP headers.
func Inject(ctx context.Context) (string, string) {
	c := propagation.MapCarrier{}
	propagator.Inject(ctx, c)
	return c.Get("traceparent"), c.Get("tracestate")
}
func Extract(ctx context.Context, parent, state string) context.Context {
	if len(parent) > 128 || len(state) > 512 {
		return ctx
	}
	return propagator.Extract(ctx, propagation.MapCarrier{"traceparent": parent, "tracestate": state})
}

type Record struct {
	Service        string            `json:"service"`
	Name           string            `json:"name"`
	TraceID        string            `json:"trace_id"`
	SpanID         string            `json:"span_id"`
	ParentID       string            `json:"parent_span_id,omitempty"`
	Start          int64             `json:"started_at"`
	DurationMicros int64             `json:"duration_us"`
	Attributes     map[string]string `json:"attributes"`
}

type Exporter struct {
	mu      sync.Mutex
	service string
	encoder *json.Encoder
	records []Record
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var codePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

func safeAttribute(a attribute.KeyValue) (string, string, bool) {
	k, v := string(a.Key), a.Value.AsString()
	switch k {
	case "request_id", "operation_id", "app_id", "command_id", "channel_id":
		return k, v, uuidPattern.MatchString(v)
	case "cluster_id", "tenant_id":
		return k, v, slugPattern.MatchString(v)
	case "operation_type", "result", "error_code":
		return k, v, codePattern.MatchString(v)
	}
	return "", "", false
}

func (e *Exporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range spans {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch s.Name() {
		case "hub.http", "hub.operation", "hub.command", "agent.command", "operator.reconcile":
		default:
			continue
		}
		r := Record{Service: e.service, Name: s.Name(), TraceID: s.SpanContext().TraceID().String(), SpanID: s.SpanContext().SpanID().String(), Start: s.StartTime().UnixMilli(), DurationMicros: s.EndTime().Sub(s.StartTime()).Microseconds(), Attributes: map[string]string{}}
		if s.Parent().IsValid() {
			r.ParentID = s.Parent().SpanID().String()
		}
		for _, a := range s.Attributes() {
			if k, v, ok := safeAttribute(a); ok {
				r.Attributes[k] = v
			}
		}
		// Never serialize SDK events, links, status descriptions or Resource attrs.
		if err := e.encoder.Encode(struct {
			Kind string `json:"kind"`
			Record
		}{"trace_span", r}); err != nil {
			return err
		}
		if len(e.records) == 3000 {
			copy(e.records, e.records[1:])
			e.records[len(e.records)-1] = r
		} else {
			e.records = append(e.records, r)
		}
	}
	return nil
}
func (e *Exporter) Shutdown(context.Context) error { return nil }
func (e *Exporter) Records(traceID string) []Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []Record{}
	for i := len(e.records) - 1; i >= 0 && len(out) < 100; i-- {
		r := e.records[i]
		if traceID != "" && r.TraceID != traceID {
			continue
		}
		r.Attributes = copyMap(r.Attributes)
		out = append(out, r)
	}
	return out
}
func copyMap(m map[string]string) map[string]string {
	n := map[string]string{}
	for k, v := range m {
		n[k] = v
	}
	return n
}

type Runtime struct {
	Provider *sdktrace.TracerProvider
	Exporter *Exporter
	Metrics  *Metrics
}

func New(service string, output io.Writer, clusters []string) (*Runtime, error) {
	if service != "hub" && service != "agent" && service != "operator" {
		return nil, fmt.Errorf("unsupported telemetry service")
	}
	if output == nil {
		return nil, fmt.Errorf("telemetry output required")
	}
	m, err := NewMetrics(clusters)
	if err != nil {
		return nil, err
	}
	e := &Exporter{service: service, encoder: json.NewEncoder(output)}
	p := sdktrace.NewTracerProvider(sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", "hugofleet-"+service))), sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithBatcher(e, sdktrace.WithMaxQueueSize(512), sdktrace.WithMaxExportBatchSize(64), sdktrace.WithBatchTimeout(200*time.Millisecond), sdktrace.WithExportTimeout(2*time.Second)))
	otel.SetTracerProvider(p)
	return &Runtime{p, e, m}, nil
}

type Metrics struct {
	Registry    *prometheus.Registry
	operations  *prometheus.CounterVec
	duration    *prometheus.HistogramVec
	connections *prometheus.GaugeVec
	errors      prometheus.Counter
	clusters    map[string]bool
}

func NewMetrics(clusters []string) (*Metrics, error) {
	if len(clusters) > 20 {
		return nil, fmt.Errorf("at most 20 telemetry clusters")
	}
	m := &Metrics{Registry: prometheus.NewRegistry(), clusters: map[string]bool{}}
	for _, id := range clusters {
		if !slugPattern.MatchString(id) || m.clusters[id] {
			return nil, fmt.Errorf("distinct registered cluster IDs required")
		}
		m.clusters[id] = true
	}
	m.operations = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "hugofleet_operations_total", Help: "Committed terminal operation transitions in this process."}, []string{"type", "state"})
	m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "hugofleet_operation_duration_seconds", Help: "Acceptance to committed terminal state duration.", Buckets: []float64{1, 5, 15, 30, 60, 120, 180, 300, 900, 1800}}, []string{"type", "state"})
	m.connections = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "hugofleet_agent_connections", Help: "Currently authenticated connected Workers; 0 or 1 per registered cluster."}, []string{"cluster"})
	m.errors = prometheus.NewCounter(prometheus.CounterOpts{Name: "hugofleet_reconcile_errors_total", Help: "Reconcile calls that returned an error in this process."})
	for _, c := range []prometheus.Collector{m.operations, m.duration, m.connections, m.errors} {
		if err := m.Registry.Register(c); err != nil {
			return nil, err
		}
	}
	for _, id := range clusters {
		m.connections.WithLabelValues(id).Set(0)
	}
	// The finite series exist immediately after restart, including zero outcomes.
	// Creating them does not increment counters or fabricate a histogram sample.
	for _, kind := range []string{"CREATE", "RELEASE", "ROLLBACK", "DELETE"} {
		for _, state := range []string{"SUCCEEDED", "FAILED", "TIMED_OUT"} {
			m.operations.WithLabelValues(kind, state)
			m.duration.WithLabelValues(kind, state)
		}
	}
	return m, nil
}
func (m *Metrics) Terminal(kind, state string, created, finished int64) {
	switch kind {
	case "CREATE", "RELEASE", "ROLLBACK", "DELETE":
	default:
		return
	}
	switch state {
	case "SUCCEEDED", "FAILED", "TIMED_OUT":
	default:
		return
	}
	if finished < created {
		return
	}
	m.operations.WithLabelValues(kind, state).Inc()
	m.duration.WithLabelValues(kind, state).Observe(float64(finished-created) / 1000)
}
func (m *Metrics) Connection(cluster string, connected bool) {
	if !m.clusters[cluster] {
		return
	}
	v := float64(0)
	if connected {
		v = 1
	}
	m.connections.WithLabelValues(cluster).Set(v)
}
func (m *Metrics) ReconcileError() { m.errors.Inc() }
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{Timeout: 3 * time.Second, MaxRequestsInFlight: 2})
}
