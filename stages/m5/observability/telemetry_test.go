package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestCommandsHaveIndependentTracesAndCorrectParents(t *testing.T) {
	var output bytes.Buffer
	e := &Exporter{service: "hub", encoder: json.NewEncoder(&output)}
	p := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	defer p.Shutdown(context.Background())
	tracer := p.Tracer("test")
	ids := []string{}
	for i := 0; i < 2; i++ {
		root, request := tracer.Start(context.Background(), "hub.http")
		ctx, hub := tracer.Start(root, "hub.command")
		parent, state := Inject(ctx)
		remote := Extract(context.Background(), parent, state)
		_, agent := tracer.Start(remote, "agent.command", trace.WithSpanKind(trace.SpanKindConsumer))
		if agent.SpanContext().TraceID() != hub.SpanContext().TraceID() || request.SpanContext().TraceID() != hub.SpanContext().TraceID() {
			t.Fatal("command broke the trace")
		}
		ids = append(ids, agent.SpanContext().TraceID().String())
		agent.End()
		hub.End()
		request.End()
	}
	if ids[0] == ids[1] {
		t.Fatal("different requests shared a stream trace")
	}
	records := e.Records("")
	if len(records) != 6 {
		t.Fatalf("records=%d", len(records))
	}
	for _, a := range records {
		if a.Name != "agent.command" {
			continue
		}
		found := false
		for _, h := range records {
			if h.Name == "hub.command" && h.TraceID == a.TraceID && h.SpanID == a.ParentID {
				found = true
			}
		}
		if !found {
			t.Fatal("Agent parent is not its command envelope")
		}
	}
}

func TestExporterDiscardsSecretsEventsAndUnboundedAttributes(t *testing.T) {
	var output bytes.Buffer
	e := &Exporter{service: "hub", encoder: json.NewEncoder(&output)}
	p := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	defer p.Shutdown(context.Background())
	_, s := p.Tracer("test").Start(context.Background(), "hub.command")
	s.SetAttributes(attribute.String("Authorization", "Bearer secret-value"), attribute.String("request_id", "secret-value"), attribute.String("operation_type", "CREATE"), attribute.String("tenant_id", "team-a"))
	s.AddEvent("secret-value", trace.WithAttributes(attribute.String("password", "secret-value")))
	s.End()
	if strings.Contains(output.String(), "secret-value") || !strings.Contains(output.String(), "team-a") {
		t.Fatal("exporter exposed secret payload or dropped allowed correlation")
	}
	r := e.Records("")
	r[0].Attributes["tenant_id"] = "mutated"
	if e.Records("")[0].Attributes["tenant_id"] != "team-a" {
		t.Fatal("viewer mutated exporter state")
	}
}

func TestMetricsRejectUnboundedLabelsAndNonTerminalStates(t *testing.T) {
	m, err := NewMetrics([]string{"hugofleet-a"})
	if err != nil {
		t.Fatal(err)
	}
	m.Terminal("arbitrary-app-name", "SUCCEEDED", 0, 1000)
	m.Terminal("CREATE", "RUNNING", 0, 1000)
	m.Terminal("CREATE", "SUCCEEDED", 1000, 3000)
	m.Connection("arbitrary-app-name", true)
	m.Connection("hugofleet-a", true)
	m.ReconcileError()
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	s := w.Body.String()
	if w.Code != 200 || strings.Contains(s, "arbitrary-app-name") || strings.Contains(s, "RUNNING") || !strings.Contains(s, `hugofleet_operations_total{state="SUCCEEDED",type="CREATE"} 1`) || !strings.Contains(s, `hugofleet_agent_connections{cluster="hugofleet-a"} 1`) || !strings.Contains(s, "hugofleet_reconcile_errors_total 1") {
		t.Fatalf("unexpected metrics:\n%s", s)
	}
}

func TestMetricsExposeZeroTerminalSeriesAfterRestart(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	for _, kind := range []string{"CREATE", "RELEASE", "ROLLBACK", "DELETE"} {
		for _, state := range []string{"SUCCEEDED", "FAILED", "TIMED_OUT"} {
			key := `hugofleet_operations_total{state="` + state + `",type="` + kind + `"} 0`
			if !strings.Contains(w.Body.String(), key) {
				t.Fatalf("missing fixed zero series %s", key)
			}
		}
	}
}
