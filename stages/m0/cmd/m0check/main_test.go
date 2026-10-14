package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKubernetesAssertionsRejectFalseSuccess(t *testing.T) {
	for _, test := range []struct {
		name  string
		args  []string
		input string
	}{
		{"missing container status", []string{"assert-broken"}, `{"status":{"phase":"Running"}}`},
		{"broken became ready", []string{"assert-broken"}, `{"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0}]}}`},
		{"liveness restarted", []string{"assert-broken"}, `{"status":{"phase":"Running","containerStatuses":[{"ready":false,"restartCount":1}]}}`},
		{"no endpoints", []string{"assert-endpoints", "broken"}, `{"items":[]}`},
		{"unknown readiness", []string{"assert-endpoints", "broken"}, `{"items":[{"endpoints":[{"conditions":{},"targetRef":{"name":"old"}}]}]}`},
		{"broken endpoint ready", []string{"assert-endpoints", "broken"}, `{"items":[{"endpoints":[{"conditions":{"ready":true},"targetRef":{"name":"broken"}}]}]}`},
		{"invalid JSON", []string{"progress"}, `{`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := run(test.args, strings.NewReader(test.input), new(bytes.Buffer)); err == nil {
				t.Fatal("invalid observation was accepted")
			}
		})
	}
	valid := `{"items":[{"endpoints":[{"conditions":{"ready":true},"targetRef":{"name":"old"}},{"conditions":{"ready":false},"targetRef":{"name":"broken"}}]}]}`
	if err := run([]string{"assert-endpoints", "broken"}, strings.NewReader(valid), new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPCheckRejectsIncorrectResponse(t *testing.T) {
	for _, test := range []struct {
		name, response string
		status         int
	}{
		{"wrong version", `{"version":"v2-good"}`, 200},
		{"HTTP error", `{"version":"v1"}`, 500},
		{"trailing JSON", `{"version":"v1"} {}`, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.response)
			}))
			defer server.Close()
			if err := checkHTTP(context.Background(), server.URL, "v1"); err == nil {
				t.Fatal("incorrect HTTP response was accepted")
			}
		})
	}
}
