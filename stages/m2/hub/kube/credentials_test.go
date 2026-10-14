package kube

import (
	clientapi "k8s.io/client-go/tools/clientcmd/api"
	"testing"
)

func restrictedConfig() *clientapi.Config {
	c := clientapi.NewConfig()
	c.CurrentContext = "hugofleet-hub"
	c.Contexts["hugofleet-hub"] = &clientapi.Context{AuthInfo: "hugofleet-hub", Cluster: "local-kind", Namespace: "hf-demo"}
	c.AuthInfos["hugofleet-hub"] = &clientapi.AuthInfo{Token: "test-token-never-used-for-network"}
	c.Clusters["local-kind"] = &clientapi.Cluster{Server: "https://127.0.0.1:16446", CertificateAuthorityData: []byte("test-inline-ca")}
	return c
}
func TestRestrictCredentialConfiguration(t *testing.T) {
	if e := validateCredentials(restrictedConfig(), "hugofleet-hub"); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name   string
		change func(*clientapi.Config)
	}{{"admin cert", func(c *clientapi.Config) { c.AuthInfos["hugofleet-hub"].ClientCertificateData = []byte("certificate") }}, {"exec plugin", func(c *clientapi.Config) {
		c.AuthInfos["hugofleet-hub"].Exec = &clientapi.ExecConfig{Command: "must-not-run"}
	}}, {"impersonation", func(c *clientapi.Config) { c.AuthInfos["hugofleet-hub"].Impersonate = "admin" }}, {"external api", func(c *clientapi.Config) { c.Clusters["local-kind"].Server = "https://remote.example:443" }}, {"insecure tls", func(c *clientapi.Config) { c.Clusters["local-kind"].InsecureSkipTLSVerify = true }}, {"proxy", func(c *clientapi.Config) { c.Clusters["local-kind"].ProxyURL = "http://proxy.example:8080" }}, {"extra context", func(c *clientapi.Config) { c.Contexts["admin"] = &clientapi.Context{} }}} {
		t.Run(tc.name, func(t *testing.T) {
			c := restrictedConfig()
			tc.change(c)
			if e := validateCredentials(c, "hugofleet-hub"); e == nil {
				t.Fatal("unrestricted credentials accepted")
			}
		})
	}
}
