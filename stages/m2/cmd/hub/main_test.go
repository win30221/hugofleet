package main

import (
	"os"
	"strings"
	"testing"
)

func TestCLIUsesIndependentFlagsAndRejectsPublicListen(t *testing.T) {
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	for _, address := range []string{"0.0.0.0:18090", "[::]:18090", "127.0.0.1:0"} {
		os.Args = []string{"hub", "--kubeconfig", "not-read", "--listen", address}
		err := run()
		if err == nil || !strings.Contains(err.Error(), "loopback") {
			t.Fatalf("expected loopback rejection for %s: %v", address, err)
		}
	}
	os.Args = []string{"hub", "--help"}
	if e := run(); e != nil {
		t.Fatal(e)
	}
}
