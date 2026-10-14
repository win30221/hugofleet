// bootstrap-hub converts a local kind administrator context into a short-lived,
// namespace-scoped ServiceAccount kubeconfig. The Hub never receives the admin file.
package main

import (
	"context"
	"flag"
	"fmt"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientapi "k8s.io/client-go/tools/clientcmd/api"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "Hub credential bootstrap failed:", e)
		os.Exit(1)
	}
}
func run() error {
	var source, dest, ctxName string
	flag.StringVar(&source, "kubeconfig", "", "script-owned kind bootstrap kubeconfig")
	flag.StringVar(&dest, "output", "", "restricted kubeconfig output")
	flag.StringVar(&ctxName, "context", "", "owned kind context")
	flag.Parse()
	switch ctxName {
	case "kind-hugofleet-a", "kind-hugofleet-m2-e2e", "kind-hugofleet-m2-demo":
	default:
		return fmt.Errorf("only dedicated HugoFleet kind contexts are allowed")
	}
	if source == "" || dest == "" {
		return fmt.Errorf("input and output paths are required")
	}
	a, e := filepath.Abs(source)
	if e != nil {
		return e
	}
	b, e := filepath.Abs(dest)
	if e != nil {
		return e
	}
	if a == b {
		return fmt.Errorf("output must not replace bootstrap kubeconfig")
	}
	raw, e := clientcmd.LoadFromFile(source)
	if e != nil {
		return e
	}
	if raw.CurrentContext != ctxName {
		return fmt.Errorf("unexpected bootstrap context")
	}
	cfg, e := clientcmd.NewNonInteractiveClientConfig(*raw, ctxName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if e != nil {
		return e
	}
	if cfg.Insecure || len(cfg.CAData) == 0 {
		return fmt.Errorf("verified inline cluster CA is required")
	}
	cfg.Timeout = 10 * time.Second
	c, e := kubernetes.NewForConfig(cfg)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	seconds := int64(8 * 60 * 60)
	token, e := c.CoreV1().ServiceAccounts("hf-demo").CreateToken(ctx, "hugofleet-hub", &authv1.TokenRequest{Spec: authv1.TokenRequestSpec{ExpirationSeconds: &seconds}}, metav1.CreateOptions{})
	if e != nil {
		return e
	}
	out := clientapi.NewConfig()
	out.CurrentContext = "hugofleet-hub"
	out.Clusters["local-kind"] = &clientapi.Cluster{Server: cfg.Host, CertificateAuthorityData: cfg.CAData}
	out.AuthInfos["hugofleet-hub"] = &clientapi.AuthInfo{Token: token.Status.Token}
	out.Contexts["hugofleet-hub"] = &clientapi.Context{Cluster: "local-kind", AuthInfo: "hugofleet-hub", Namespace: "hf-demo"}
	data, e := clientcmd.Write(*out)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(b), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(b, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return e
	}
	_, e = f.Write(data)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Println("Restricted Hub credentials written; expires", token.Status.ExpirationTimestamp.UTC().Format(time.RFC3339))
	return nil
}
