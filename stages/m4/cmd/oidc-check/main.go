package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/win30221/hugofleet/stages/m4/democonfig"
	"github.com/win30221/hugofleet/stages/m4/testdriver"
	"os"
	"time"
)

func main() {
	f := flag.NewFlagSet("oidc-check", flag.ExitOnError)
	settingsPath := f.String("settings", "", "private generated accounts JSON")
	output := f.String("output", "", "private verified subjects JSON")
	f.Parse(os.Args[1:])
	if err := run(*settingsPath, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(settingsPath, output string) error {
	if settingsPath == "" || output == "" {
		return fmt.Errorf("private settings and output files required")
	}
	settings, err := testdriver.ReadSettings(settingsPath)
	if err != nil {
		return err
	}
	subjects := map[string]string{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	for _, account := range settings.Accounts {
		subject, err := testdriver.LearnSubject(ctx, settings.OIDC, account)
		if err != nil {
			return fmt.Errorf("%s: %w", account.Name, err)
		}
		subjects[account.Name] = subject
		fmt.Printf("PASS real Dex Code + PKCE/JWKS/issuer/audience/expiry/nonce for %s; subject retained privately.\n", account.Name)
	}
	return democonfig.WritePrivate(output, subjects)
}
