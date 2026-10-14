// agentcheck waits for actual Worker Pod mTLS sessions reported by the Hub.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	url := flag.String("url", "http://127.0.0.1:18090", "local Hub endpoint")
	tokenFile := flag.String("token-file", "", "private Demo token file")
	clusters := flag.String("clusters", "hugofleet-a,hugofleet-b", "expected registered Worker sessions")
	flag.Parse()
	token, err := os.ReadFile(*tokenFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	hc := &http.Client{Timeout: 3 * time.Second}
	var last error
	for {
		r, err := http.NewRequestWithContext(ctx, "GET", *url+"/api/v1/clusters", nil)
		if err != nil {
			return err
		}
		r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
		res, err := hc.Do(r)
		if err == nil {
			var cs []domain.Cluster
			if res.StatusCode == 200 {
				err = json.NewDecoder(res.Body).Decode(&cs)
			} else {
				err = fmt.Errorf("management API returned %d", res.StatusCode)
			}
			res.Body.Close()
			if err == nil {
				for _, id := range strings.Split(*clusters, ",") {
					found := false
					for _, c := range cs {
						if c.ID == id && c.State == "CONNECTED" && c.AgentVersion != "" && c.LastSeenAt > 0 {
							found = true
						}
					}
					if !found {
						err = fmt.Errorf("Worker %s has no active mTLS Agent session", id)
						break
					}
				}
			}
			if err == nil {
				fmt.Println("PASS actual Worker Pod mTLS sessions:", *clusters)
				return nil
			}
		}
		last = err
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("Agent connection check: %w; last=%v", ctx.Err(), last)
		case <-timer.C:
		}
	}
}
