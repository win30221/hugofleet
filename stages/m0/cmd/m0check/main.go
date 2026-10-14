// m0check verifies real M0 HTTP responses and Kubernetes JSON captured by kubectl.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"
)

type pod struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Containers []struct {
			Image string `json:"image"`
		} `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase             string `json:"phase"`
		ContainerStatuses []struct {
			Ready        bool `json:"ready"`
			RestartCount int  `json:"restartCount"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run(args []string, input io.Reader, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: m0check http URL VERSION | retired-pods IMAGE | broken-pod | progress | assert-broken | assert-endpoints POD | client-version")
	}
	if args[0] == "http" {
		if len(args) != 3 {
			return errors.New("http requires URL and VERSION")
		}
		if err := checkHTTP(context.Background(), args[1], args[2]); err != nil {
			return err
		}
		fmt.Fprintf(output, "PASS HTTP %s: health, readiness, version, quote=8000\n", args[2])
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(input, 8<<20))
	switch args[0] {
	case "retired-pods", "broken-pod":
		if (args[0] == "retired-pods" && len(args) != 2) || (args[0] == "broken-pod" && len(args) != 1) {
			return errors.New("retired-pods requires IMAGE; broken-pod takes no arguments")
		}
		var list struct {
			Items []pod `json:"items"`
		}
		if err := decoder.Decode(&list); err != nil {
			return err
		}
		for _, p := range list.Items {
			if len(p.Spec.Containers) != 1 {
				return errors.New("expected one pricing-api container per Pod")
			}
			image := p.Spec.Containers[0].Image
			if args[0] == "retired-pods" && image != args[1] {
				fmt.Fprintln(output, "pod/"+p.Metadata.Name)
			}
			if args[0] == "broken-pod" && image == "pricing-api:v2-broken" && p.Status.Phase == "Running" {
				fmt.Fprintln(output, p.Metadata.Name)
				break
			}
		}
	case "progress":
		var deployment struct {
			Status struct {
				Conditions []struct{ Type, Status, Reason string } `json:"conditions"`
			} `json:"status"`
		}
		if err := decoder.Decode(&deployment); err != nil {
			return err
		}
		for _, c := range deployment.Status.Conditions {
			if c.Type == "Progressing" && c.Status == "False" {
				fmt.Fprintln(output, c.Reason)
			}
		}
	case "assert-broken":
		var p pod
		if err := decoder.Decode(&p); err != nil {
			return err
		}
		if p.Status.Phase != "Running" || len(p.Status.ContainerStatuses) != 1 {
			return errors.New("broken Pod must be Running with one container status")
		}
		status := p.Status.ContainerStatuses[0]
		if status.Ready || status.RestartCount != 0 {
			return errors.New("broken readiness must fail without liveness restarts")
		}
		fmt.Fprintln(output, "PASS Kubernetes: broken rollout stalled; readiness false; no liveness restarts")
	case "assert-endpoints":
		if len(args) != 2 {
			return errors.New("assert-endpoints requires broken POD name")
		}
		var slices struct {
			Items []struct {
				Endpoints []struct {
					Conditions struct {
						Ready *bool `json:"ready"`
					} `json:"conditions"`
					TargetRef struct {
						Name string `json:"name"`
					} `json:"targetRef"`
				} `json:"endpoints"`
			} `json:"items"`
		}
		if err := decoder.Decode(&slices); err != nil {
			return err
		}
		ready := 0
		for _, slice := range slices.Items {
			for _, endpoint := range slice.Endpoints {
				if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready {
					if endpoint.TargetRef.Name == args[1] {
						return errors.New("broken Pod included in ready Service endpoints")
					}
					ready++
				}
			}
		}
		if ready == 0 {
			return errors.New("Service must retain at least one ready endpoint")
		}
		fmt.Fprintln(output, "PASS Service: ready endpoints retained; broken Pod excluded")
	case "client-version":
		var version struct {
			ClientVersion struct {
				GitVersion string `json:"gitVersion"`
			} `json:"clientVersion"`
		}
		if err := decoder.Decode(&version); err != nil {
			return err
		}
		if version.ClientVersion.GitVersion == "" {
			return errors.New("missing kubectl client version")
		}
		fmt.Fprintln(output, version.ClientVersion.GitVersion)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	return nil
}

func checkHTTP(ctx context.Context, baseURL, version string) error {
	if version != "v1" && version != "v2-good" && version != "v2-broken" {
		return errors.New("unsupported version")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("HTTP check requires a loopback http://127.0.0.1:PORT URL")
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	baseURL = strings.TrimSuffix(baseURL, "/")
	request := func(path string, wantStatus int, body []byte) (map[string]any, error) {
		method := http.MethodGet
		if body != nil {
			method = http.MethodPost
		}
		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		defer response.Body.Close()
		if response.StatusCode != wantStatus {
			return nil, fmt.Errorf("%s: got HTTP %d, want %d", path, response.StatusCode, wantStatus)
		}
		var observed map[string]any
		decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
		if err := decoder.Decode(&observed); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: unexpected trailing response", path)
		}
		return observed, nil
	}
	observed, err := request("/version", http.StatusOK, nil)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(observed, map[string]any{"version": version}) {
		return fmt.Errorf("unexpected version: %v", observed)
	}
	if _, err := request("/healthz", http.StatusOK, nil); err != nil {
		return err
	}
	readyStatus := http.StatusOK
	if version == "v2-broken" {
		readyStatus = http.StatusServiceUnavailable
	}
	if _, err := request("/readyz", readyStatus, nil); err != nil {
		return err
	}
	quote, err := request("/quote", http.StatusOK, []byte(`{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}`))
	if err != nil {
		return err
	}
	expected := map[string]any{"original_amount_cents": float64(20000), "tier_discount_cents": float64(2000), "coupon_discount_cents": float64(10000), "final_amount_cents": float64(8000)}
	if !reflect.DeepEqual(quote, expected) {
		return fmt.Errorf("unexpected quote: %v", quote)
	}
	return nil
}
