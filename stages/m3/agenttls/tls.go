package agenttls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
)

const ServerName = "hub.hugofleet.local"
const IdentityPrefix = "spiffe://hugofleet.local/cluster/"

func Config(dir, name string, server bool) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key"))
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, fmt.Errorf("invalid CA")
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}
	if server {
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	} else {
		cfg.RootCAs = pool
		cfg.ServerName = ServerName
	}
	return cfg, nil
}
func Cluster(cert *x509.Certificate) (string, error) {
	if len(cert.URIs) != 1 {
		return "", fmt.Errorf("exactly one cluster URI SAN required")
	}
	u := cert.URIs[0]
	if u.Scheme != "spiffe" || u.Host != "hugofleet.local" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("invalid cluster URI SAN")
	}
	for _, id := range []string{"hugofleet-a", "hugofleet-b", "hugofleet-m3-e2e-a", "hugofleet-m3-e2e-b", "hugofleet-m4-e2e-a", "hugofleet-m4-e2e-b", "hugofleet-m5-e2e-a", "hugofleet-m5-e2e-b"} {
		if u.String() == IdentityPrefix+id {
			return id, nil
		}
	}
	return "", fmt.Errorf("unregistered cluster identity")
}
