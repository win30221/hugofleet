package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/win30221/hugofleet/stages/m4/testdriver"
	"os"
	"time"
)

func main() {
	path := flag.String("settings", "", "private generated local Demo accounts")
	flag.Parse()
	if flag.NArg() != 0 || *path == "" {
		fmt.Fprintln(os.Stderr, "explicit private Demo settings required")
		os.Exit(1)
	}
	v, err := testdriver.ReadSettings(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !v.OIDC.LocalDemo {
		fmt.Fprintln(os.Stderr, "generated local Demo only")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, a := range v.Accounts {
		s, err := testdriver.LoginHub(ctx, v.OIDC, a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "verified Demo enrollment failed for %s: %v\n", a.Name, err)
			os.Exit(1)
		}
		if status, _, err := s.Request(ctx, "POST", "/logout", "", nil); err != nil || status != 303 {
			fmt.Fprintln(os.Stderr, "Demo enrollment logout failed")
			os.Exit(1)
		}
		fmt.Printf("Verified real OIDC User %s; no tenant membership granted.\n", a.Name)
	}
}
