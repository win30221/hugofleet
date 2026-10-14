package agenttls

import (
	"crypto/x509"
	"net/url"
	"testing"
)

func TestExplicitStage5IdentityAndNoURIAdoption(t *testing.T) {
	for _, id := range []string{"hugofleet-m5-e2e-a", "hugofleet-m5-e2e-b"} {
		u, err := url.Parse(IdentityPrefix + id)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Cluster(&x509.Certificate{URIs: []*url.URL{u}})
		if err != nil || got != id {
			t.Fatalf("explicit M5 SAN rejected: %q %v", got, err)
		}
	}
	for _, raw := range []string{IdentityPrefix + "foreign", IdentityPrefix + "hugofleet-m5-e2e-a?admin=true", IdentityPrefix + "hugofleet-m5-e2e-a#admin", IdentityPrefix + "hugofleet-m5-e2e-a/../hugofleet-b", IdentityPrefix + "hugofleet-m5-e2e-%61", "spiffe://other.local/cluster/hugofleet-m5-e2e-a"} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = Cluster(&x509.Certificate{URIs: []*url.URL{u}}); err == nil {
			t.Fatalf("unexpected URI adopted: %s", raw)
		}
	}
}
