package domain

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
	"strings"
)

// The immutable tenant ID is its DNS slug. No client-supplied namespace exists.
func NamespaceForTenant(tenant string) string { return "hf-" + tenant }
func ValidateTenant(tenant string) error {
	if len(tenant) == 0 || len(tenant) > 60 || len(validation.IsDNS1123Label(tenant)) != 0 {
		return Problem("INVALID_ARGUMENT", "tenant must be a DNS label of 1..60 characters")
	}
	return nil
}

type Actor struct{ UserID, SessionHash, RequestID string }
type actorKey struct{}

func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}
func ActorContext(ctx context.Context) Actor { actor, _ := ctx.Value(actorKey{}).(Actor); return actor }

type User struct {
	ID            string `json:"id"`
	Issuer        string `json:"issuer"`
	Subject       string `json:"subject"`
	Email         string `json:"email"`
	PlatformAdmin bool   `json:"platform_admin"`
	CreatedAt     int64  `json:"created_at"`
}
type Membership struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}
type TenantProfile struct {
	TenantID     string            `json:"tenant_id"`
	ClusterID    string            `json:"cluster_id"`
	Namespace    string            `json:"namespace"`
	BootstrapUID string            `json:"bootstrap_uid"`
	Hard         map[string]string `json:"hard"`
	Defaults     map[string]string `json:"defaults"`
}

func (p TenantProfile) Validate() error {
	if err := ValidateTenant(p.TenantID); err != nil {
		return err
	}
	if p.Namespace != NamespaceForTenant(p.TenantID) || p.ClusterID == "" || p.BootstrapUID == "" || len(p.Hard) != 5 || len(p.Defaults) != 4 {
		return Problem("INVALID_ARGUMENT", "incomplete or mismatched Worker tenant profile")
	}
	for _, key := range []string{"requests.cpu", "requests.memory", "limits.cpu", "limits.memory", "pods"} {
		q, err := resource.ParseQuantity(p.Hard[key])
		if err != nil || q.Sign() <= 0 {
			return Problem("INVALID_ARGUMENT", "invalid quota "+key)
		}
	}
	pods := resource.MustParse(p.Hard["pods"])
	if pods.Value() > 100 || pods.AsDec().String() != fmt.Sprint(pods.Value()) {
		return Problem("INVALID_ARGUMENT", "pods quota must be an integer of 1..100")
	}
	for _, key := range []string{"requests.cpu", "requests.memory", "limits.cpu", "limits.memory"} {
		q, err := resource.ParseQuantity(p.Defaults[key])
		if err != nil || q.Sign() <= 0 {
			return Problem("INVALID_ARGUMENT", "invalid default "+key)
		}
	}
	for _, suffix := range []string{"cpu", "memory"} {
		rq := resource.MustParse(p.Hard["requests."+suffix])
		lq := resource.MustParse(p.Hard["limits."+suffix])
		rd := resource.MustParse(p.Defaults["requests."+suffix])
		ld := resource.MustParse(p.Defaults["limits."+suffix])
		if rq.Cmp(lq) > 0 || rd.Cmp(ld) > 0 || rd.Cmp(rq) > 0 || ld.Cmp(lq) > 0 {
			return Problem("INVALID_ARGUMENT", "inconsistent resource profile")
		}
	}
	return nil
}

type AuditEvent struct {
	ID         string `json:"id"`
	ActorID    string `json:"actor_id"`
	TenantID   string `json:"tenant_id,omitempty"`
	Action     string `json:"action"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Outcome    string `json:"outcome"`
	RequestID  string `json:"request_id"`
	OccurredAt int64  `json:"occurred_at"`
}

func ValidRole(role string) bool { return role == "Owner" || role == "Developer" || role == "Viewer" }
func ValidDisplayName(name string) bool {
	return len(name) > 0 && len(name) <= 100 && strings.TrimSpace(name) == name && !strings.ContainsAny(name, "\r\n\x00")
}
