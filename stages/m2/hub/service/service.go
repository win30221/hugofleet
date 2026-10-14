package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"go.opentelemetry.io/otel/attribute"
	"log/slog"
	"strings"
	"time"
)

// Transport accepts only a validated, versioned Application intent.
type Transport interface {
	Ping(context.Context) error
	Apply(context.Context, domain.Application, platform.ApplicationSpec) (string, error)
	Observe(context.Context, domain.Application, platform.ApplicationSpec) (domain.Observation, error)
	Delete(context.Context, domain.Application) (domain.Observation, error)
}
type ClusterTransport interface {
	ClusterIDs() []string
	PingCluster(context.Context, string) error
	ClusterState(string) (bool, int64, string)
}
type operationContextKey struct{}

func OperationContext(ctx context.Context) (domain.Operation, bool) {
	o, ok := ctx.Value(operationContextKey{}).(domain.Operation)
	return o, ok
}

type Service struct {
	Store   *repository.Store
	Policy  domain.Policy
	Timeout time.Duration
}

func (s *Service) accept(ctx context.Context, c domain.Change, payload any) (domain.Accepted, error) {
	if len(c.Key) < 1 || len(c.Key) > 128 || strings.TrimSpace(c.Key) != c.Key || strings.ContainsAny(c.Key, "\r\n") {
		return domain.Accepted{}, domain.Problem("INVALID_ARGUMENT", "Idempotency-Key is required and must contain 1..128 characters")
	}
	raw, err := json.Marshal(struct {
		Tenant, App, Kind string
		Payload           any
	}{c.TenantID, c.AppID, c.Kind, payload})
	if err != nil {
		return domain.Accepted{}, err
	}
	h := sha256.Sum256(raw)
	c.Hash = hex.EncodeToString(h[:])
	return s.Store.Accept(ctx, c, s.Timeout)
}
func (s *Service) Deploy(ctx context.Context, tenant, app, key string, r domain.DeployRequest) (domain.Accepted, error) {
	if r.ExpectedDesiredRevision == nil || *r.ExpectedDesiredRevision < 0 {
		return domain.Accepted{}, domain.Problem("INVALID_ARGUMENT", "expected_desired_revision is required and must be nonnegative")
	}
	kind := "RELEASE"
	if app == "" {
		kind = "CREATE"
		if err := domain.ValidateName(r.Name); err != nil {
			return domain.Accepted{}, err
		}
	} else if r.Name != "" || r.ClusterID != "" {
		return domain.Accepted{}, domain.Problem("INVALID_ARGUMENT", "release cannot rename or migrate an application")
	}
	spec, validationErr := s.Policy.Spec(r)
	return s.accept(ctx, domain.Change{TenantID: tenant, AppID: app, Name: r.Name, ClusterID: r.ClusterID, Key: key, Kind: kind, Expected: *r.ExpectedDesiredRevision, Spec: spec, ValidateSpec: func(platform.ApplicationSpec) error { return validationErr }}, r)
}
func (s *Service) Rollback(ctx context.Context, tenant, app, key string, r domain.RollbackRequest) (domain.Accepted, error) {
	if r.ExpectedDesiredRevision == nil || *r.ExpectedDesiredRevision < 1 || r.TargetRevision < 1 {
		return domain.Accepted{}, domain.Problem("INVALID_ARGUMENT", "positive target_revision and expected_desired_revision are required")
	}
	return s.accept(ctx, domain.Change{TenantID: tenant, AppID: app, Key: key, Kind: "ROLLBACK", Expected: *r.ExpectedDesiredRevision, SourceRevision: r.TargetRevision, ValidateSpec: func(spec platform.ApplicationSpec) error {
		_, err := s.Policy.Spec(domain.DeployRequest{Image: spec.Image, Replicas: spec.Replicas, Port: spec.Port, Resources: spec.Resources, ReadinessPath: spec.ReadinessPath, LivenessPath: spec.LivenessPath})
		return err
	}}, r)
}
func (s *Service) Delete(ctx context.Context, tenant, app, key string, r domain.DeleteRequest) (domain.Accepted, error) {
	if r.ExpectedDesiredRevision == nil || *r.ExpectedDesiredRevision < 1 {
		return domain.Accepted{}, domain.Problem("INVALID_ARGUMENT", "positive expected_desired_revision is required")
	}
	return s.accept(ctx, domain.Change{TenantID: tenant, AppID: app, Key: key, Kind: "DELETE", Expected: *r.ExpectedDesiredRevision}, r)
}

type Runner struct {
	Connection     func(string, bool)
	ReconcileError func()
	Store          *repository.Store
	Transport      Transport
	Interval       time.Duration
}

func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		if err := r.Tick(ctx); err != nil && ctx.Err() == nil {
			if r.ReconcileError != nil {
				r.ReconcileError()
			}
			slog.Error("Hub reconciliation failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (r *Runner) Tick(ctx context.Context) error {
	connections := map[string]bool{}
	if remote, ok := r.Transport.(ClusterTransport); ok {
		for _, id := range remote.ClusterIDs() {
			connected, seen, version := remote.ClusterState(id)
			connections[id] = connected
			if r.Connection != nil {
				r.Connection(id, connected)
			}
			if err := r.Store.ClusterConnectivity(ctx, id, connected, seen, version); err != nil {
				return err
			}
		}
	} else {
		probe, cancel := context.WithTimeout(ctx, 5*time.Second)
		connected := r.Transport.Ping(probe) == nil
		cancel()
		if err := r.Store.Connectivity(ctx, connected); err != nil {
			return err
		}
		connections[domain.ClusterID] = connected
	}
	ops, err := r.Store.Pending(ctx, 30)
	if err != nil {
		return err
	}
	for _, o := range ops {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		a, e := r.Store.App(ctx, o.AppID)
		if e != nil {
			return e
		}
		if err = r.step(ctx, o, connections[a.ClusterID]); err != nil {
			if r.ReconcileError != nil {
				r.ReconcileError()
			}
			slog.Error("Operation reconciliation will retry", "request_id", o.RequestID, "operation_id", o.ID, "cluster_id", a.ClusterID, "tenant_id", a.TenantID, "app_id", a.ID, "error", err)
		}
	}
	// At most 30 live apps; retained deletion history cannot starve observations.
	apps, err := r.Store.ObservationApps(ctx)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return nil
	}
	for _, a := range apps {
		if a.Status == "DELETED" || a.Status == "DELETING" {
			continue
		}
		if !connections[a.ClusterID] {
			if err = r.Store.Unknown(ctx, a); err != nil {
				return err
			}
			continue
		}
		rel, e := r.Store.Release(ctx, a.ID, a.DesiredRevision)
		if e != nil {
			return e
		}
		opCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		ob, e := r.Transport.Observe(opCtx, a, rel.Spec)
		stop()
		if e != nil {
			var problem *domain.Error
			if errors.As(e, &problem) {
				if err = r.Store.Observe(ctx, a, domain.Observation{ErrorCode: problem.Code, Message: problem.Message}); err != nil {
					return err
				}
				continue
			}
			if e = r.Store.Unknown(ctx, a); e != nil {
				return e
			}
			continue
		}
		if err = r.Store.Observe(ctx, a, ob); err != nil {
			return err
		}
	}
	return nil
}
func (r *Runner) step(ctx context.Context, o domain.Operation, connected bool) error {
	a, err := r.Store.App(ctx, o.AppID)
	if err != nil {
		return err
	}
	if time.Now().UnixMilli() >= o.DeadlineAt {
		return r.Store.Finish(ctx, o, a, "TIMED_OUT", "DEADLINE_EXCEEDED", "Hub deadline expired; Kubernetes may still continue the rollout")
	}
	if !connected {
		return nil
	}
	ctx = observability.Extract(ctx, o.Traceparent, o.Tracestate)
	ctx, span := observability.Tracer().Start(ctx, "hub.operation")
	span.SetAttributes(attribute.String("request_id", o.RequestID), attribute.String("operation_id", o.ID), attribute.String("cluster_id", a.ClusterID), attribute.String("tenant_id", a.TenantID), attribute.String("app_id", a.ID), attribute.String("operation_type", o.Type))
	defer span.End()
	if err = r.Store.Dispatch(ctx, o.ID); err != nil {
		return err
	}
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	opCtx = context.WithValue(opCtx, operationContextKey{}, o)
	var ob domain.Observation
	if o.Type == "DELETE" {
		ob, err = r.Transport.Delete(opCtx, a)
		if err == nil && ob.UID != "" && a.KubernetesUID == "" {
			err = r.Store.BindDeletionUID(ctx, o, a, ob.UID)
		}
	} else {
		rel, e := r.Store.Release(ctx, a.ID, a.DesiredRevision)
		if e != nil {
			return e
		}
		if rel.ID != o.ReleaseID {
			return fmt.Errorf("operation does not match current desired revision")
		}
		var uid string
		uid, err = r.Transport.Apply(opCtx, a, rel.Spec)
		if err == nil {
			if err = r.Store.Applied(ctx, o, a, uid); err != nil {
				return err
			}
			a.KubernetesUID = uid
			ob, err = r.Transport.Observe(opCtx, a, rel.Spec)
		}
	}
	// The durable operation deadline takes precedence over a late RPC error.
	if time.Now().UnixMilli() >= o.DeadlineAt {
		return r.Store.Finish(ctx, o, a, "TIMED_OUT", "DEADLINE_EXCEEDED", "Hub deadline expired while observing Kubernetes")
	}
	if err != nil {
		if p, ok := err.(*domain.Error); ok {
			return r.Store.Finish(ctx, o, a, "FAILED", p.Code, p.Message)
		}
		return err
	}
	// Re-check persisted deadline after slow API calls before considering success.
	if time.Now().UnixMilli() >= o.DeadlineAt {
		return r.Store.Finish(ctx, o, a, "TIMED_OUT", "DEADLINE_EXCEEDED", "Hub deadline expired while observing Kubernetes")
	}
	if ob.ErrorCode != "" {
		return r.Store.Finish(ctx, o, a, "FAILED", ob.ErrorCode, ob.Message, ob)
	}
	if ob.Ready || ob.Deleted {
		return r.Store.Finish(ctx, o, a, "SUCCEEDED", "", "Desired Kubernetes state verified", ob)
	}
	return nil
}
