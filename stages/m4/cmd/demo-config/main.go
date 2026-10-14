package main

import (
	"flag"
	"fmt"
	"github.com/win30221/hugofleet/stages/m4/democonfig"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	f := flag.NewFlagSet("m4-demo-config", flag.ExitOnError)
	dir := f.String("output", "", "private project runtime directory")
	issuer := f.String("issuer", "http://127.0.0.1:5556/dex", "fixed loopback Dex issuer")
	base := f.String("base-url", "http://127.0.0.1:18094", "fixed loopback Hub URL")
	f.Parse(os.Args[1:])
	if err := run(*dir, *issuer, *base); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(dir, issuer, base string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(filepath.Join(root, ".local"), absolute)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("Demo output must remain in a named project .local directory")
	}
	if err = democonfig.Generate(absolute, issuer, base); err != nil {
		return err
	}
	fmt.Printf("Generated private real Dex accounts/config in %s; no credentials printed.\n", absolute)
	return nil
}
