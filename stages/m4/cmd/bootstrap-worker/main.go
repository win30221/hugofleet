package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/win30221/hugofleet/stages/m4/bootstrap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"net/url"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap failed:", err)
		os.Exit(1)
	}
}
func run() error {
	f := flag.NewFlagSet("bootstrap-worker", flag.ContinueOnError)
	cluster := f.String("cluster", "", "exact project Worker identity")
	kubeconfig := f.String("kubeconfig", "", "administrator's local owned Worker kubeconfig (never sent to Hub)")
	action := f.String("action", "tenant", "tenant, receipt, agent or endpoint (M4 only)")
	stage := f.String("stage", "m4", "m4 direct install or m5 Helm security preparation")
	tenant := f.String("tenant", "", "approved immutable tenant DNS slug")
	tenants := f.String("tenants", "", "Agent trusted comma-separated tenant allowlist")
	endpoint := f.String("hub", "", "resolved Hub Docker network IP:port")
	certs := f.String("certs", "", "private directory containing Worker-only credentials")
	requestCPU := f.String("requests-cpu", "1", "namespace requests.cpu quota")
	requestMemory := f.String("requests-memory", "1Gi", "namespace requests.memory quota")
	limitCPU := f.String("limits-cpu", "3", "namespace limits.cpu quota")
	limitMemory := f.String("limits-memory", "2Gi", "namespace limits.memory quota")
	pods := f.String("pods", "8", "namespace Pod quota")
	if err := f.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if *stage != "m4" && *stage != "m5" {
		return fmt.Errorf("unsupported bootstrap stage")
	}
	switch *cluster {
	case "hugofleet-a", "hugofleet-b", "hugofleet-m4-e2e-a", "hugofleet-m4-e2e-b":
	case "hugofleet-m5-e2e-a", "hugofleet-m5-e2e-b":
		if *stage != "m5" {
			return fmt.Errorf("M5 Worker requires M5 preparation")
		}
	default:
		return fmt.Errorf("unsupported owned project Worker")
	}
	if *kubeconfig == "" {
		return fmt.Errorf("explicit administrator kubeconfig required")
	}
	raw, err := clientcmd.LoadFromFile(*kubeconfig)
	if err != nil {
		return err
	}
	expected := "kind-" + *cluster
	if raw.CurrentContext != expected || raw.Contexts[expected] == nil {
		return fmt.Errorf("kubeconfig context must be exactly %s", expected)
	}
	cfg, err := clientcmd.NewNonInteractiveClientConfig(*raw, expected, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return err
	}
	target, err := url.Parse(cfg.Host)
	if err != nil || target.Scheme != "https" || target.Hostname() != "127.0.0.1" || cfg.Insecure {
		return fmt.Errorf("bootstrap only accepts the verified loopback kind API endpoint")
	}
	cfg.Timeout = 10 * time.Second
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme, networkingv1.AddToScheme, rbacv1.AddToScheme} {
		if err = add(scheme); err != nil {
			return err
		}
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	p := &bootstrap.Provisioner{Client: c, Cluster: *cluster, PrepareOnly: *stage == "m5"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	switch *action {
	case "endpoint":
		return p.UpdateAgentEndpoint(ctx, *endpoint)
	case "tenant", "receipt":
		profile := bootstrap.DefaultProfile(*tenant, *cluster)
		profile.Hard = map[string]string{"requests.cpu": *requestCPU, "requests.memory": *requestMemory, "limits.cpu": *limitCPU, "limits.memory": *limitMemory, "pods": *pods}
		if *action == "receipt" {
			err = p.FinalizeHelmTenant(ctx, profile)
		} else {
			err = p.ProvisionTenant(ctx, profile)
		}
		if err != nil {
			return err
		}
		if *stage == "m5" {
			fmt.Printf("M5 owned %s/%s %s preparation complete; Hub activation still required.\n", *cluster, profile.Namespace, *action)
		} else {
			fmt.Printf("Provisioned owned %s/%s: quota, limits, namespaced RBAC, Operator and ingress policy; Hub activation still required.\n", *cluster, profile.Namespace)
		}
	case "agent":
		credentials := map[string][]byte{}
		for key, name := range map[string]string{"ca.crt": "ca.crt", "client.crt": *cluster + ".crt", "client.key": *cluster + ".key"} {
			raw, err := os.ReadFile(filepath.Join(*certs, name))
			if err != nil {
				return fmt.Errorf("missing Worker credential %s", name)
			}
			if len(raw) > 16<<10 {
				return fmt.Errorf("oversized Worker credential")
			}
			credentials[key] = raw
		}
		if err = p.InstallAgent(ctx, strings.Split(*tenants, ","), *endpoint, credentials); err != nil {
			return err
		}
		if *stage == "m5" {
			fmt.Printf("Prepared owned M5 Worker-only TLS and persistent ledger in %s/%s; Helm installs the Agent.\n", *cluster, bootstrap.ControlNamespace)
		} else {
			fmt.Printf("Installed restricted Agent %s/%s with explicit tenant allowlist; verify real mTLS connection before use.\n", *cluster, bootstrap.ControlNamespace)
		}
	default:
		return fmt.Errorf("action must be tenant, receipt, agent or endpoint")
	}
	return nil
}
