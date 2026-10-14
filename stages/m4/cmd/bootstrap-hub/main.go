package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/win30221/hugofleet/stages/m3/devcert"
	"github.com/win30221/hugofleet/stages/m4/auth"
	"github.com/win30221/hugofleet/stages/m4/democonfig"
	"github.com/win30221/hugofleet/stages/m4/testdriver"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	f := flag.NewFlagSet("bootstrap-oidc-hub", flag.ExitOnError)
	state := f.String("state", "", "private generated Demo directory")
	ids := f.String("clusters", "hugofleet-a,hugofleet-b", "registered Worker identities")
	dial := f.String("issuer-dial", "", "optional exact private Dex IP:port for Docker Hub")
	peer := f.String("http-client-ip", "", "optional exact Docker management gateway")
	stage := f.String("stage", "m4", "m4 or m5 Worker registry")
	f.Parse(os.Args[1:])
	if err := run(*state, *ids, *dial, *peer, *stage); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(state, ids, dial, peer, stage string) error {
	if stage != "m4" && stage != "m5" {
		return fmt.Errorf("unsupported Hub bootstrap stage")
	}
	settings, err := testdriver.ReadSettings(filepath.Join(state, "accounts.json"))
	if err != nil {
		return err
	}
	file, err := os.Open(filepath.Join(state, "subjects.json"))
	if err != nil {
		return fmt.Errorf("first verify real Dex Code + PKCE subjects")
	}
	var subjects map[string]string
	err = json.NewDecoder(io.LimitReader(file, 16<<10)).Decode(&subjects)
	closeErr := file.Close()
	if err != nil || closeErr != nil || subjects["admin"] == "" {
		return fmt.Errorf("verified Admin subject missing")
	}
	clusters := strings.Split(ids, ",")
	if len(clusters) != 2 || clusters[0] == clusters[1] {
		return fmt.Errorf("two distinct cluster identities required")
	}
	for _, id := range clusters {
		switch id {
		case "hugofleet-a", "hugofleet-b", "hugofleet-m4-e2e-a", "hugofleet-m4-e2e-b":
		case "hugofleet-m5-e2e-a", "hugofleet-m5-e2e-b":
			if stage != "m5" {
				return fmt.Errorf("M5 Worker requires M5 bootstrap")
			}
		default:
			return fmt.Errorf("unsupported project Worker")
		}
	}
	settings.OIDC.IssuerDial = dial
	settings.OIDC.HTTPClientIP = peer
	if err = settings.OIDC.Validate(); err != nil {
		return err
	}
	credentials := filepath.Join(state, "certs")
	if _, err = os.Stat(filepath.Join(credentials, "ca.crt")); os.IsNotExist(err) {
		if err = devcert.GenerateMTLS(credentials, clusters); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	cfg := struct {
		OIDC         auth.Config `json:"oidc"`
		AdminSubject string      `json:"admin_subject"`
		Clusters     []string    `json:"clusters"`
	}{settings.OIDC, subjects["admin"], clusters}
	if err = democonfig.WritePrivate(filepath.Join(state, "hub-config.json"), cfg); err != nil {
		return err
	}
	fmt.Println("Prepared OIDC Hub config bound to verified issuer/sub; no Worker kubeconfig or Demo admin token.")
	return nil
}
