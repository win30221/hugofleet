package kube

import (
	"context"
	"fmt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	authzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientapi "k8s.io/client-go/tools/clientcmd/api"
	"net/url"
)

func validateCredentials(raw *clientapi.Config, contextName string) error {
	if contextName != "hugofleet-hub" || raw.CurrentContext != contextName || len(raw.Contexts) != 1 || len(raw.Clusters) != 1 || len(raw.AuthInfos) != 1 {
		return fmt.Errorf("Hub requires the single bootstrap-generated hugofleet-hub context")
	}
	ctx := raw.Contexts[contextName]
	if ctx == nil || ctx.AuthInfo != "hugofleet-hub" || ctx.Cluster != domain.ClusterID || ctx.Namespace != domain.Namespace {
		return fmt.Errorf("unexpected restricted Hub context")
	}
	user := raw.AuthInfos[ctx.AuthInfo]
	cluster := raw.Clusters[ctx.Cluster]
	if user == nil || user.Token == "" || user.TokenFile != "" || user.Exec != nil || user.AuthProvider != nil || user.Username != "" || user.Password != "" || user.ClientCertificate != "" || len(user.ClientCertificateData) != 0 || user.ClientKey != "" || len(user.ClientKeyData) != 0 || user.Impersonate != "" || user.ImpersonateUID != "" || len(user.ImpersonateGroups) != 0 || len(user.ImpersonateUserExtra) != 0 {
		return fmt.Errorf("Hub accepts only the bootstrap ServiceAccount token, without executable credential plugins or administrator certificates")
	}
	if cluster == nil || cluster.InsecureSkipTLSVerify || len(cluster.CertificateAuthorityData) == 0 || cluster.CertificateAuthority != "" || cluster.ProxyURL != "" || cluster.TLSServerName != "" {
		return fmt.Errorf("Hub requires verified inline cluster CA without proxy overrides")
	}
	endpoint, err := url.Parse(cluster.Server)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return fmt.Errorf("M2 Hub accepts only the loopback HTTPS kind endpoint")
	}
	return nil
}
func localConfig(path, contextName string) (*rest.Config, error) {
	raw, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return nil, err
	}
	if err = validateCredentials(raw, contextName); err != nil {
		return nil, err
	}
	return clientcmd.NewNonInteractiveClientConfig(*raw, contextName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
}
func (t *Transport) verifyAccess(ctx context.Context) error {
	t.accessMu.Lock()
	defer t.accessMu.Unlock()
	if t.accessVerified {
		return nil
	}
	for _, tenant := range t.tenants {
		namespace := domain.NamespaceForTenant(tenant)
		for _, test := range []struct {
			verb, group, resource, sub, ns string
			allowed                        bool
		}{{"create", "platform.hugofleet.io", "applications", "", namespace, true}, {"get", "", "secrets", "", namespace, false}, {"create", "", "pods", "exec", namespace, false}, {"create", "platform.hugofleet.io", "applications", "", "default", false}} {
			review, err := t.auth.SelfSubjectAccessReviews().Create(ctx, &authzv1.SelfSubjectAccessReview{Spec: authzv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authzv1.ResourceAttributes{Verb: test.verb, Group: test.group, Resource: test.resource, Subresource: test.sub, Namespace: test.ns}}}, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			if review.Status.Allowed != test.allowed {
				return domain.Problem("PERMISSION_DENIED", "Hub credentials do not match the limited development permissions")
			}
		}
	}
	t.accessVerified = true
	return nil
}
