// Package devcert creates local, short-lived demo credentials using crypto/x509.
package devcert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"github.com/win30221/hugofleet/stages/m3/agenttls"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

func Generate(dir string, clusters []string) error {
	return generate(dir, clusters, true)
}

// GenerateMTLS omits M3's temporary management token. M4 uses OIDC exclusively.
func GenerateMTLS(dir string, clusters []string) error { return generate(dir, clusters, false) }
func generate(dir string, clusters []string, managementToken bool) error {
	// Refuse to silently rotate a running deployment's CA or overwrite its token.
	if _, e := os.Stat(filepath.Join(dir, "ca.crt")); e == nil {
		return fmt.Errorf("credentials already exist; use a fresh directory for explicit rotation")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	serial := func() *big.Int {
		n, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if e != nil {
			panic(e)
		}
		return n
	}
	now := time.Now()
	caKey, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return e
	}
	ca := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "HugoFleet local demo CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	raw, e := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "ca.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}), 0600); e != nil {
		return e
	}
	// The CA private key is intentionally not persisted. Rotation reissues the set.
	for _, name := range append([]string{"hub"}, clusters...) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		c := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: name}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
		if name == "hub" {
			c.DNSNames = []string{agenttls.ServerName}
			c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		} else {
			u, err := url.Parse(agenttls.IdentityPrefix + name)
			if err != nil {
				return err
			}
			c.URIs = []*url.URL{u}
			c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		}
		b, err := x509.CreateCertificate(rand.Reader, c, ca, &key.PublicKey, caKey)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, name+".crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: b}), 0600); err != nil {
			return err
		}
		b, err = x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, name+".key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b}), 0600); err != nil {
			return err
		}
	}
	if !managementToken {
		return nil
	}
	token := make([]byte, 32)
	if _, e = rand.Read(token); e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(dir, "admin-token"), []byte(hex.EncodeToString(token)), 0600)
}
