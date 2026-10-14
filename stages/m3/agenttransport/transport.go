package agenttransport

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/google/uuid"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/service"
	"github.com/win30221/hugofleet/stages/m3/agentprotocol"
	"github.com/win30221/hugofleet/stages/m3/agenttls"
	pb "github.com/win30221/hugofleet/stages/m3/api/agent/v1"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"sync"
	"time"
)

type session struct {
	channelID   string
	queue       chan *pb.HubEnvelope
	done        chan struct{}
	pending     map[string]chan *pb.CommandResult
	last        time.Time
	version     string
	maxDeadline int64
}
type Transport struct {
	pb.UnimplementedAgentControlServiceServer
	mu               sync.Mutex
	ids              []string
	sessions         map[string]*session
	quarantine       map[string]int64
	seen             map[string]int64
	versions         map[string]string
	HeartbeatTimeout time.Duration
}

func New(ids []string) *Transport {
	return &Transport{ids: append([]string(nil), ids...), sessions: map[string]*session{}, quarantine: map[string]int64{}, seen: map[string]int64{}, versions: map[string]string{}, HeartbeatTimeout: 15 * time.Second}
}
func (t *Transport) Server(cfg *tls.Config) *grpc.Server {
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(cfg)), grpc.MaxRecvMsgSize(128<<10), grpc.MaxSendMsgSize(128<<10), grpc.MaxConcurrentStreams(8))
	pb.RegisterAgentControlServiceServer(server, t)
	return server
}
func (t *Transport) Connect(stream grpc.BidiStreamingServer[pb.AgentEnvelope, pb.HubEnvelope]) error {
	p, ok := peer.FromContext(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing TLS peer")
	}
	auth, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(auth.State.VerifiedChains) == 0 || len(auth.State.PeerCertificates) == 0 {
		return status.Error(codes.Unauthenticated, "verified mTLS required")
	}
	id, err := agenttls.Cluster(auth.State.PeerCertificates[0])
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	registered := false
	for _, v := range t.ids {
		if v == id {
			registered = true
		}
	}
	if !registered {
		return status.Error(codes.PermissionDenied, "cluster not registered")
	}
	incoming := make(chan *pb.AgentEnvelope, 1)
	failed := make(chan error, 1)
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	go func() {
		for {
			v, e := stream.Recv()
			if e != nil {
				select {
				case failed <- e:
				case <-ctx.Done():
				}
				return
			}
			select {
			case incoming <- v:
			case <-ctx.Done():
				return
			}
		}
	}()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	var hello *pb.AgentHello
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-failed:
		return err
	case <-timer.C:
		return status.Error(codes.DeadlineExceeded, "hello timeout")
	case v := <-incoming:
		hello = v.GetHello()
	}
	if hello == nil || hello.ClusterId != id || len(hello.AgentVersion) > 64 {
		return status.Error(codes.PermissionDenied, "hello must match certificate cluster identity")
	}
	s := &session{queue: make(chan *pb.HubEnvelope, 8), done: make(chan struct{}), pending: map[string]chan *pb.CommandResult{}, last: time.Now(), version: hello.AgentVersion, channelID: uuid.NewString()}
	t.mu.Lock()
	if t.sessions[id] != nil || t.quarantine[id] > time.Now().UnixMilli() {
		t.mu.Unlock()
		return status.Error(codes.AlreadyExists, "cluster already has a live or draining session")
	}
	t.sessions[id] = s
	t.seen[id] = s.last.UnixMilli()
	t.versions[id] = s.version
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.sessions, id)
		t.quarantine[id] = s.maxDeadline
		close(s.done)
		t.mu.Unlock()
	}()
	if err = stream.Send(&pb.HubEnvelope{Payload: &pb.HubEnvelope_Welcome{Welcome: &pb.HubWelcome{SessionId: s.channelID}}}); err != nil {
		return err
	}
	// One stream writer; a stalled HTTP/2 peer cannot block heartbeat expiry.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case c := <-s.queue:
				if c.ExpiresAt <= time.Now().UnixMilli() {
					continue
				}
				if e := stream.Send(c); e != nil {
					select {
					case failed <- e:
					case <-ctx.Done():
					}
					return
				}
			}
		}
	}()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err = <-failed:
			return err
		case <-ticker.C:
			t.mu.Lock()
			expired := time.Since(s.last) > t.HeartbeatTimeout
			t.mu.Unlock()
			if expired {
				return status.Error(codes.DeadlineExceeded, "heartbeat expired")
			}
		case event := <-incoming:
			t.mu.Lock()
			if event.GetHeartbeat() != nil {
				s.last = time.Now()
				t.seen[id] = s.last.UnixMilli()
			}
			if result := event.GetResult(); result != nil {
				if done := s.pending[result.CommandId]; done != nil {
					select {
					case done <- result:
					default:
					}
				}
			}
			t.mu.Unlock()
			if event.GetHello() != nil {
				return status.Error(codes.InvalidArgument, "duplicate hello")
			}
		}
	}
}
func (t *Transport) ClusterIDs() []string { return append([]string(nil), t.ids...) }
func (t *Transport) ClusterState(id string) (bool, int64, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.sessions[id]
	return s != nil && time.Since(s.last) <= t.HeartbeatTimeout, t.seen[id], t.versions[id]
}
func (t *Transport) PingCluster(ctx context.Context, id string) error {
	connected, _, _ := t.ClusterState(id)
	if !connected {
		return fmt.Errorf("cluster %s disconnected", id)
	}
	return ctx.Err()
}
func (t *Transport) Ping(ctx context.Context) error {
	for _, id := range t.ids {
		if e := t.PingCluster(ctx, id); e != nil {
			return e
		}
	}
	return nil
}
func (t *Transport) callResult(ctx context.Context, id string, c *pb.HubEnvelope) (*pb.CommandResult, error) {
	ctx, span := observability.Tracer().Start(ctx, "hub.command")
	defer span.End()
	span.SetAttributes(attribute.String("cluster_id", id), attribute.String("operation_id", c.OperationId), attribute.String("request_id", c.RequestId), attribute.String("command_id", c.CommandId))
	if v := c.GetApply(); v != nil {
		span.SetAttributes(attribute.String("app_id", v.GetApplication().GetApplicationId()), attribute.String("tenant_id", v.GetApplication().GetTenantId()))
	}
	if v := c.GetDelete(); v != nil {
		span.SetAttributes(attribute.String("app_id", v.GetApplication().GetApplicationId()), attribute.String("tenant_id", v.GetApplication().GetTenantId()))
	}
	if v := c.GetSnapshot(); v != nil {
		span.SetAttributes(attribute.String("app_id", v.GetApplication().GetApplicationId()), attribute.String("tenant_id", v.GetApplication().GetTenantId()))
	}
	c.Traceparent, c.Tracestate = observability.Inject(ctx)
	c.PayloadHash = agentprotocol.PayloadHash(c)
	ch := make(chan *pb.CommandResult, 1)
	t.mu.Lock()
	s := t.sessions[id]
	if s == nil || time.Since(s.last) > t.HeartbeatTimeout {
		t.mu.Unlock()
		return nil, fmt.Errorf("worker disconnected")
	}
	if len(s.pending) >= 8 || s.pending[c.CommandId] != nil {
		t.mu.Unlock()
		return nil, fmt.Errorf("worker command queue full or duplicate in flight")
	}
	span.SetAttributes(attribute.String("channel_id", s.channelID))
	s.pending[c.CommandId] = ch
	s.maxDeadline = max(s.maxDeadline, c.ExpiresAt)
	t.mu.Unlock()
	defer func() { t.mu.Lock(); delete(s.pending, c.CommandId); t.mu.Unlock() }()
	select {
	case s.queue <- c:
	case <-s.done:
		return nil, fmt.Errorf("session disconnected")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case r := <-ch:
		if r.ErrorCode == "UNAVAILABLE" || r.ErrorCode == "EXPIRED" {
			return nil, fmt.Errorf("worker Kubernetes unavailable")
		}
		if r.ErrorCode != "" {
			return nil, domain.Problem(r.ErrorCode, r.ErrorMessage)
		}
		return r, nil
	case <-s.done:
		return nil, fmt.Errorf("session disconnected")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (t *Transport) Call(ctx context.Context, id string, c *pb.HubEnvelope) (domain.Observation, error) {
	r, err := t.callResult(ctx, id, c)
	if err != nil {
		return domain.Observation{}, err
	}
	if r.Status == nil {
		return domain.Observation{}, fmt.Errorf("worker result has no status")
	}
	var expected *pb.ApplicationIdentity
	if v := c.GetApply(); v != nil {
		expected = v.Application
	}
	if v := c.GetDelete(); v != nil {
		expected = v.Application
	}
	if v := c.GetSnapshot(); v != nil {
		expected = v.Application
	}
	actual := r.Status.Application
	if expected == nil || actual == nil || actual.ApplicationId != expected.ApplicationId || actual.Name != expected.Name || actual.Namespace != expected.Namespace || actual.TenantId != expected.TenantId || actual.Revision != expected.Revision || (expected.Uid != "" && actual.Uid != "" && actual.Uid != expected.Uid) {
		return domain.Observation{}, domain.Problem("OWNERSHIP_CONFLICT", "Agent result identity does not match the command")
	}
	if actual.Uid != r.Status.Uid {
		return domain.Observation{}, fmt.Errorf("inconsistent Worker UID report")
	}
	if r.Status.Ready && (!r.Status.Applied || r.Status.Uid == "" || r.Status.Generation < 1 || r.Status.ObservedRevision != expected.Revision || r.Status.ObservedGeneration != r.Status.Generation) {
		return domain.Observation{}, fmt.Errorf("Worker readiness has no matching revision/generation evidence")
	}
	return agentprotocol.Observation(r.Status), nil
}

func (t *Transport) TenantProfile(ctx context.Context, cluster, tenant string) (domain.TenantProfile, error) {
	if err := domain.ValidateTenant(tenant); err != nil {
		return domain.TenantProfile{}, err
	}
	c, _ := envelope(ctx, false)
	c.Payload = &pb.HubEnvelope_TenantProfile{TenantProfile: &pb.RequestTenantProfile{TenantId: tenant}}
	r, err := t.callResult(ctx, cluster, c)
	if err != nil {
		return domain.TenantProfile{}, err
	}
	if r.TenantProfile == nil {
		return domain.TenantProfile{}, fmt.Errorf("Worker did not report a tenant profile")
	}
	p := agentprotocol.TenantProfile(r.TenantProfile)
	if err = p.Validate(); err != nil {
		return domain.TenantProfile{}, err
	}
	if p.ClusterID != cluster || p.TenantID != tenant {
		return domain.TenantProfile{}, domain.Problem("OWNERSHIP_CONFLICT", "Worker tenant profile identity mismatch")
	}
	return p, nil
}
func envelope(ctx context.Context, mutate bool) (*pb.HubEnvelope, error) {
	expires := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(expires) {
		expires = d
	}
	c := &pb.HubEnvelope{CommandId: uuid.NewString(), ExpiresAt: expires.UnixMilli()}
	if op, ok := service.OperationContext(ctx); ok {
		c.OperationId = op.ID
		c.RequestId = op.RequestID
	}
	if mutate {
		op, ok := service.OperationContext(ctx)
		if !ok {
			return nil, fmt.Errorf("durable operation context required")
		}
		c.CommandId = op.CommandID
		c.OperationId = op.ID
		c.ExpiresAt = min(c.ExpiresAt, op.DeadlineAt)
	}
	return c, nil
}
func (t *Transport) Observe(ctx context.Context, a domain.Application, spec platform.ApplicationSpec) (domain.Observation, error) {
	c, _ := envelope(ctx, false)
	c.Payload = &pb.HubEnvelope_Snapshot{Snapshot: &pb.RequestSnapshot{Application: agentprotocol.Identity(a), Spec: agentprotocol.Spec(spec)}}
	return t.Call(ctx, a.ClusterID, c)
}
func (t *Transport) Apply(ctx context.Context, a domain.Application, spec platform.ApplicationSpec) (string, error) {
	// Always read Worker state before retrying a durable mutation (including restart).
	ob, err := t.Observe(ctx, a, spec)
	if err != nil {
		return "", err
	}
	if ob.Applied {
		return ob.UID, nil
	}
	if ob.ErrorCode != "" && ob.ErrorCode != "APPLICATION_MISSING" && ob.ErrorCode != "SPEC_DRIFT" {
		return "", domain.Problem(ob.ErrorCode, ob.Message)
	}
	c, err := envelope(ctx, true)
	if err != nil {
		return "", err
	}
	c.Payload = &pb.HubEnvelope_Apply{Apply: &pb.ApplyApplication{Application: agentprotocol.Identity(a), Spec: agentprotocol.Spec(spec)}}
	ob, err = t.Call(ctx, a.ClusterID, c)
	return ob.UID, err
}
func (t *Transport) Delete(ctx context.Context, a domain.Application) (domain.Observation, error) {
	c, _ := envelope(ctx, false)
	c.Payload = &pb.HubEnvelope_Snapshot{Snapshot: &pb.RequestSnapshot{Application: agentprotocol.Identity(a)}}
	ob, err := t.Call(ctx, a.ClusterID, c)
	if err != nil || ob.Deleted || a.KubernetesUID == "" {
		return ob, err
	}
	c, err = envelope(ctx, true)
	if err != nil {
		return ob, err
	}
	c.Payload = &pb.HubEnvelope_Delete{Delete: &pb.DeleteApplication{Application: agentprotocol.Identity(a)}}
	return t.Call(ctx, a.ClusterID, c)
}
