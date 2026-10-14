package traceview

import (
	"bytes"
	"strings"
	"testing"
)

func TestCollectedViewerFiltersPrivatePayloadAndEscapesOutput(t *testing.T) {
	input := `{"kind":"trace_span","service":"hub","name":"hub.command","trace_id":"11111111111111111111111111111111","span_id":"2222222222222222","started_at":1,"duration_us":2,"attributes":{"tenant_id":"team-a","Authorization":"Bearer test-secret","request_id":"test-secret"},"events":[{"password":"test-secret"}]}`
	items, err := Read(strings.NewReader(input), 10)
	if err != nil || len(items) != 1 {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err = Render(&output, items); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "test-secret") || !strings.Contains(output.String(), "team-a") {
		t.Fatal("private log payload reached standalone viewer")
	}
}
