package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m3/agentprotocol"
	pb "github.com/win30221/hugofleet/stages/m3/api/agent/v1"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"go.opentelemetry.io/otel/attribute"
	corev1 "k8s.io/api/core/v1"
	"log/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
	"time"
)

const LedgerName = "hugofleet-agent-ledger"

type Entry struct {
	TenantID    string `json:"tenant_id,omitempty"`
	Revision    int64  `json:"revision"`
	SpecHash    string `json:"spec_hash"`
	UID         string `json:"uid"`
	Deleted     bool   `json:"deleted"`
	CommandID   string `json:"command_id"`
	CommandHash string `json:"command_hash"`
	Name        string `json:"name"`
}
type Ledger interface {
	Load(context.Context) (map[string]Entry, error)
	Save(context.Context, map[string]Entry) error
}

// KubeLedger has access only to the pre-created, named ConfigMap. Kubernetes
// resourceVersion protects every write; an API conflict is retried next command.
type KubeLedger struct {
	Namespace string
	Client    client.Client
	current   *corev1.ConfigMap
}

func (l *KubeLedger) Load(ctx context.Context) (map[string]Entry, error) {
	cm := &corev1.ConfigMap{}
	namespace := l.Namespace
	if namespace == "" {
		namespace = domain.Namespace
	}
	if err := l.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: LedgerName}, cm); err != nil {
		return nil, err
	}
	entries := map[string]Entry{}
	if raw := cm.Data["ledger.json"]; raw != "" {
		if e := json.Unmarshal([]byte(raw), &entries); e != nil {
			return nil, e
		}
	}
	l.current = cm
	return entries, nil
}
func (l *KubeLedger) Save(ctx context.Context, entries map[string]Entry) error {
	raw, e := json.Marshal(entries)
	if e != nil {
		return e
	}
	// Fail closed before Kubernetes' 1 MiB limit; never prune tombstones.
	if len(raw) > 512<<10 {
		return domain.Problem("LEDGER_FULL", "worker ledger is full; manual controlled migration required")
	}
	if l.current == nil {
		return fmt.Errorf("ledger must be loaded before saving")
	}
	cm := l.current.DeepCopy()
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["ledger.json"] = string(raw)
	if e = l.Client.Update(ctx, cm); e != nil {
		return e
	}
	l.current = cm
	return nil
}

type Workloads interface {
	Apply(context.Context, domain.Application, platform.ApplicationSpec) (string, error)
	Observe(context.Context, domain.Application, platform.ApplicationSpec) (domain.Observation, error)
	Delete(context.Context, domain.Application) (domain.Observation, error)
	DeletionStatus(context.Context, domain.Application) (domain.Observation, error)
}
type Engine struct {
	Profiles interface {
		ReadProfile(context.Context, string) (domain.TenantProfile, error)
	}
	Tenants   map[string]bool
	Cluster   string
	Ledger    Ledger
	Workloads Workloads
	mu        sync.Mutex
}

func (e *Engine) Execute(ctx context.Context, c *pb.HubEnvelope) *pb.CommandResult {
	ctx = observability.Extract(ctx, c.GetTraceparent(), c.GetTracestate())
	ctx, span := observability.Tracer().Start(ctx, "agent.command")
	defer span.End()
	span.SetAttributes(attribute.String("cluster_id", e.Cluster), attribute.String("operation_id", c.GetOperationId()), attribute.String("request_id", c.GetRequestId()), attribute.String("command_id", c.GetCommandId()))
	span.SetAttributes(attribute.String("channel_id", observability.ChannelContext(ctx)))
	e.mu.Lock()
	defer e.mu.Unlock()
	result := &pb.CommandResult{CommandId: c.GetCommandId()}
	defer func() {
		span.SetAttributes(attribute.String("error_code", result.ErrorCode))
		if v := result.GetStatus().GetApplication(); v != nil {
			span.SetAttributes(attribute.String("app_id", v.GetApplicationId()), attribute.String("tenant_id", v.GetTenantId()))
		}
		if span.IsRecording() {
			correlation := observability.Correlation{ClusterID: e.Cluster, OperationID: c.GetOperationId(), RequestID: c.GetRequestId()}
			if v := result.GetStatus().GetApplication(); v != nil {
				correlation.AppID = v.GetApplicationId()
				correlation.TenantID = v.GetTenantId()
			}
			attrs := observability.LogAttributes(correlation)
			attrs = append(attrs, "trace_id", span.SpanContext().TraceID().String(), "error_code", result.ErrorCode, "channel_id", observability.ChannelContext(ctx))
			slog.Info("Worker command completed", attrs...)
		}
	}()
	var ob domain.Observation
	var err error
	if c.GetTenantProfile() != nil {
		var p domain.TenantProfile
		p, err = e.profile(ctx, c)
		if err == nil {
			result.TenantProfile = agentprotocol.Profile(p)
		}
	} else {
		ob, err = e.execute(ctx, c)
	}
	if err != nil {
		var p *domain.Error
		if errors.As(err, &p) {
			result.ErrorCode = p.Code
			result.ErrorMessage = p.Message
		} else {
			result.ErrorCode = "UNAVAILABLE"
			result.ErrorMessage = "Worker Kubernetes operation unavailable"
		}
	} else if c.GetTenantProfile() == nil {
		result.Status = agentprotocol.Status(ob)
		var id *pb.ApplicationIdentity
		if v := c.GetApply(); v != nil {
			id = v.Application
		}
		if v := c.GetDelete(); v != nil {
			id = v.Application
		}
		if v := c.GetSnapshot(); v != nil {
			id = v.Application
		}
		if id != nil {
			result.Status.Application = agentprotocol.Identity(agentprotocol.App(id, e.Cluster))
			result.Status.Application.Uid = ob.UID
		}
	}
	return result
}
func (e *Engine) execute(ctx context.Context, c *pb.HubEnvelope) (domain.Observation, error) {
	empty := domain.Observation{}
	if c.GetCommandId() == "" || len(c.CommandId) > 128 || c.ExpiresAt <= time.Now().UnixMilli() {
		return empty, domain.Problem("EXPIRED", "command deadline expired or command identity invalid")
	}
	if c.PayloadHash == "" || c.PayloadHash != agentprotocol.PayloadHash(c) {
		return empty, domain.Problem("INVALID_ARGUMENT", "payload hash mismatch")
	}
	var id *pb.ApplicationIdentity
	var wire *pb.ApplicationSpec
	switch {
	case c.GetApply() != nil:
		id = c.GetApply().Application
		wire = c.GetApply().Spec
	case c.GetDelete() != nil:
		id = c.GetDelete().Application
	case c.GetSnapshot() != nil:
		id = c.GetSnapshot().Application
		wire = c.GetSnapshot().Spec
	default:
		return empty, domain.Problem("INVALID_ARGUMENT", "unsupported command")
	}
	if id == nil || !e.tenantAllowed(id.TenantId) || id.Namespace != domain.NamespaceForTenant(id.TenantId) || id.ApplicationId == "" || len(id.ApplicationId) > 128 || id.Revision < 1 || domain.ValidateName(id.Name) != nil {
		return empty, domain.Problem("PERMISSION_DENIED", "invalid application identity or namespace")
	}
	a := agentprotocol.App(id, e.Cluster)
	ctx = observability.WithCorrelation(ctx, observability.Correlation{RequestID: c.RequestId, OperationID: c.OperationId, ClusterID: e.Cluster, TenantID: a.TenantID, AppID: a.ID})
	deadline, cancel := context.WithDeadline(ctx, time.UnixMilli(c.ExpiresAt))
	defer cancel()
	ctx = deadline
	entries, err := e.Ledger.Load(ctx)
	if err != nil {
		return empty, err
	}
	old, exists := entries[a.ID]
	oldTenant := old.TenantID
	if oldTenant == "" {
		oldTenant = domain.TenantID
	}
	if exists && (oldTenant != a.TenantID || old.Name != a.Name || (a.KubernetesUID != "" && old.UID != "" && old.UID != a.KubernetesUID)) {
		return empty, domain.Problem("OWNERSHIP_CONFLICT", "worker ledger identity differs")
	}
	if a.KubernetesUID == "" {
		a.KubernetesUID = old.UID
	}
	if c.GetSnapshot() != nil {
		if wire == nil {
			return e.Workloads.DeletionStatus(ctx, a)
		}
		spec := agentprotocol.Platform(wire, id.Revision)
		if err = spec.Validate(); err != nil {
			return empty, domain.Problem("INVALID_ARGUMENT", err.Error())
		}
		return e.Workloads.Observe(ctx, a, spec)
	}
	if c.OperationId == "" {
		return empty, domain.Problem("INVALID_ARGUMENT", "mutation requires operation_id")
	}
	slog.Info("Worker mutation received", "cluster_id", e.Cluster, "tenant_id", a.TenantID, "app_id", a.ID, "operation_id", c.OperationId, "request_id", c.RequestId, "command_id", c.CommandId, "revision", a.DesiredRevision)
	if exists && old.CommandID == c.CommandId && old.CommandHash != c.PayloadHash {
		return empty, domain.Problem("CONFLICT", "same command identity has a different payload")
	}
	if exists && old.Revision > a.DesiredRevision {
		return empty, domain.Problem("STALE_REVISION", "command revision is older than worker high-water mark")
	}
	if c.GetDelete() != nil {
		if a.KubernetesUID == "" {
			return empty, domain.Problem("INVALID_ARGUMENT", "delete requires persisted Worker CR UID precondition")
		}
		// Read and verify identity before recording an irreversible tombstone.
		ob, err := e.Workloads.DeletionStatus(ctx, a)
		if err != nil {
			return empty, err
		}
		if exists && old.Deleted && (old.CommandID != c.CommandId || old.CommandHash != c.PayloadHash) {
			return empty, domain.Problem("CONFLICT", "application already tombstoned by another command")
		}
		entries[a.ID] = Entry{TenantID: a.TenantID, Revision: a.DesiredRevision, UID: a.KubernetesUID, Deleted: true, CommandID: c.CommandId, CommandHash: c.PayloadHash, Name: a.Name}
		if err = e.Ledger.Save(ctx, entries); err != nil {
			return empty, err
		}
		if ob.Deleted {
			return ob, nil
		}
		if ctx.Err() != nil {
			return empty, ctx.Err()
		}
		return e.Workloads.Delete(ctx, a)
	}
	if old.Deleted {
		return empty, domain.Problem("DELETED", "worker tombstone refuses resurrection")
	}
	spec := agentprotocol.Platform(wire, id.Revision)
	if err = spec.Validate(); err != nil {
		return empty, domain.Problem("INVALID_ARGUMENT", err.Error())
	}
	hash := agentprotocol.Hash(spec)
	if exists && old.Revision == spec.Revision && old.SpecHash != hash {
		return empty, domain.Problem("CONFLICT", "same revision has a different configuration")
	}
	entries[a.ID] = Entry{TenantID: a.TenantID, Revision: spec.Revision, SpecHash: hash, UID: old.UID, CommandID: c.CommandId, CommandHash: c.PayloadHash, Name: a.Name}
	if err = e.Ledger.Save(ctx, entries); err != nil {
		return empty, err
	}
	if ctx.Err() != nil {
		return empty, ctx.Err()
	}
	uid, err := e.Workloads.Apply(ctx, a, spec)
	if err != nil {
		return empty, err
	}
	entry := entries[a.ID]
	entry.UID = uid
	entries[a.ID] = entry
	if err = e.Ledger.Save(ctx, entries); err != nil {
		return empty, err
	}
	return domain.Observation{UID: uid, Applied: true}, nil
}

func (e *Engine) tenantAllowed(tenant string) bool {
	if e.Tenants == nil {
		return tenant == domain.TenantID
	}
	return e.Tenants[tenant] && domain.ValidateTenant(tenant) == nil
}

func (e *Engine) profile(ctx context.Context, c *pb.HubEnvelope) (domain.TenantProfile, error) {
	if c.CommandId == "" || len(c.CommandId) > 128 || c.ExpiresAt <= time.Now().UnixMilli() {
		return domain.TenantProfile{}, domain.Problem("EXPIRED", "profile request expired")
	}
	if c.PayloadHash == "" || c.PayloadHash != agentprotocol.PayloadHash(c) {
		return domain.TenantProfile{}, domain.Problem("INVALID_ARGUMENT", "payload hash mismatch")
	}
	tenant := c.GetTenantProfile().TenantId
	if e.Profiles == nil || !e.tenantAllowed(tenant) {
		return domain.TenantProfile{}, domain.Problem("PERMISSION_DENIED", "tenant profile is not approved on this Worker")
	}
	bounded, cancel := context.WithDeadline(ctx, time.UnixMilli(c.ExpiresAt))
	defer cancel()
	p, err := e.Profiles.ReadProfile(bounded, tenant)
	if err != nil {
		return p, err
	}
	if err = p.Validate(); err != nil {
		return domain.TenantProfile{}, err
	}
	if p.ClusterID != e.Cluster || p.TenantID != tenant {
		return domain.TenantProfile{}, domain.Problem("OWNERSHIP_CONFLICT", "profile identity mismatch")
	}
	return p, nil
}
