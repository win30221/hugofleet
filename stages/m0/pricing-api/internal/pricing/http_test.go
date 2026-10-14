package pricing

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuoteHTTP(t *testing.T) {
	handler, err := NewHandler(Config{Version: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, body, contentType string
		status                  int
	}{
		{"valid", `{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}`, "application/json; charset=utf-8", 200},
		{"missing amount", `{"customer_tier":"standard"}`, "application/json", 422},
		{"null amount", `{"amount_cents":null,"customer_tier":"standard"}`, "application/json", 422},
		{"negative", `{"amount_cents":-1,"customer_tier":"standard"}`, "application/json", 422},
		{"maximum exceeded", `{"amount_cents":1000000000001,"customer_tier":"standard"}`, "application/json", 422},
		{"decimal", `{"amount_cents":1.5,"customer_tier":"standard"}`, "application/json", 400},
		{"integer overflow", `{"amount_cents":9223372036854775808,"customer_tier":"standard"}`, "application/json", 400},
		{"string amount", `{"amount_cents":"100","customer_tier":"standard"}`, "application/json", 400},
		{"unknown field", `{"amount_cents":100,"customer_tier":"standard","extra":1}`, "application/json", 400},
		{"unknown tier", `{"amount_cents":100,"customer_tier":"gold"}`, "application/json", 422},
		{"unknown coupon", `{"amount_cents":100,"customer_tier":"vip","coupon_code":"BAD"}`, "application/json", 422},
		{"invalid JSON", `{`, "application/json", 400},
		{"null document", `null`, "application/json", 422},
		{"array", `[]`, "application/json", 400},
		{"trailing JSON", `{"amount_cents":100,"customer_tier":"standard"} {}`, "application/json", 400},
		{"missing content type", `{}`, "", 415},
		{"wrong content type", `{}`, "text/plain", 415},
		{"oversized", strings.Repeat(" ", MaxBodyBytes+1), "application/json", 413},
		{"oversized suffix", `{"amount_cents":1,"customer_tier":"standard"}` + strings.Repeat(" ", MaxBodyBytes), "application/json", 413},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/quote", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body, test.status)
			}
			if response.Header().Get("X-Request-ID") == "" {
				t.Fatal("missing request ID")
			}
			if test.status == 200 {
				var quote Quote
				if err := json.Unmarshal(response.Body.Bytes(), &quote); err != nil {
					t.Fatal(err)
				}
				if quote != (Quote{20_000, 2_000, 10_000, 8_000}) {
					t.Fatalf("unexpected quote: %+v", quote)
				}
			} else {
				var failure errorResponse
				if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
					t.Fatal(err)
				}
				if failure.Error.Code == "" || failure.Error.RequestID != response.Header().Get("X-Request-ID") {
					t.Fatalf("missing or mismatched error details: %+v", failure)
				}
			}
		})
	}
}

func TestVersionsAndProbes(t *testing.T) {
	for _, version := range []string{"v1", "v2-good", "v2-broken"} {
		t.Run(version, func(t *testing.T) {
			handler, err := NewHandler(Config{Version: version})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			for _, path := range []string{"/healthz", "/readyz", "/version"} {
				response, err := server.Client().Get(server.URL + path)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				wantStatus := http.StatusOK
				if version == "v2-broken" && path == "/readyz" {
					wantStatus = http.StatusServiceUnavailable
				}
				if response.StatusCode != wantStatus {
					t.Fatalf("%s status = %d; want %d", path, response.StatusCode, wantStatus)
				}
				if path == "/version" && !strings.Contains(string(body), `"version":"`+version+`"`) {
					t.Fatalf("unexpected version: %s", body)
				}
			}
		})
	}
}

func TestMethodShutdownAndLogging(t *testing.T) {
	var logs bytes.Buffer
	handler, err := NewHandler(Config{Version: "v1", Ready: func() bool { return false },
		Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/quote", 405}, {"POST", "/healthz", 405}, {"POST", "/readyz", 405},
		{"POST", "/version", 405}, {"GET", "/readyz", 503}, {"GET", "/missing", 404},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, nil)
		request.Header.Set("Authorization", "Bearer do-not-log-this-secret")
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("%s %s status = %d; want %d", test.method, test.path, response.Code, test.status)
		}
		if response.Code == 405 && response.Header().Get("Allow") == "" {
			t.Fatal("missing Allow header")
		}
	}
	if strings.Contains(logs.String(), "do-not-log-this-secret") || !strings.Contains(logs.String(), "request_id") {
		t.Fatalf("unexpected logs: %s", logs.String())
	}
	if _, err := NewHandler(Config{Version: "unknown"}); err == nil {
		t.Fatal("invalid build version accepted")
	}
}
