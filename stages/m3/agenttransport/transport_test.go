package agenttransport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m3/agent"
	"github.com/win30221/hugofleet/stages/m3/agentprotocol"
	"github.com/win30221/hugofleet/stages/m3/agenttls"
	pb "github.com/win30221/hugofleet/stages/m3/api/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"math/big"
	"net"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
	"time"
)

type authority struct {
	key  *ecdsa.PrivateKey
	cert *x509.Certificate
	pool *x509.CertPool
}

func ca(t *testing.T) *authority {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	raw, e := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	cert, e = x509.ParseCertificate(raw)
	if e != nil {
		t.Fatal(e)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &authority{key, cert, pool}
}
func (a *authority) issue(t *testing.T, id, dns string, expired bool) tls.Certificate {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	if expired {
		cert.NotAfter = time.Now().Add(-time.Minute)
	}
	if dns != "" {
		cert.DNSNames = []string{dns}
		cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	} else {
		u, _ := url.Parse(agenttls.IdentityPrefix + id)
		cert.URIs = []*url.URL{u}
		cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	raw, e := x509.CreateCertificate(rand.Reader, cert, a.cert, &key.PublicKey, a.key)
	if e != nil {
		t.Fatal(e)
	}
	return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key}
}
func startRPC(t *testing.T, a *authority, dns string) (*Transport, string) {
	t.Helper()
	transport := New([]string{"hugofleet-a", "hugofleet-b"})
	transport.HeartbeatTimeout = 150 * time.Millisecond
	server := transport.Server(&tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: a.pool, Certificates: []tls.Certificate{a.issue(t, "", dns, false)}})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve(listener)
	t.Cleanup(func() { server.Stop(); listener.Close() })
	return transport, listener.Addr().String()
}
func connect(t *testing.T, addr string, cfg *tls.Config, id string) (grpc.BidiStreamingClient[pb.AgentEnvelope, pb.HubEnvelope], context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	conn, e := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { cancel(); conn.Close() })
	stream, e := pb.NewAgentControlServiceClient(conn).Connect(ctx)
	if e != nil {
		t.Fatal(e)
	}
	// A send may succeed before the TLS handshake fails; Recv must reject it.
	stream.Send(&pb.AgentEnvelope{Payload: &pb.AgentEnvelope_Hello{Hello: &pb.AgentHello{ClusterId: id, AgentVersion: "test"}}})
	return stream, cancel
}
func TestRealTLSRejectsUntrustedCAWrongSANExpiredAndIdentity(t *testing.T) {
	trusted := ca(t)
	foreign := ca(t)
	_, addr := startRPC(t, trusted, agenttls.ServerName)
	denied := func(t *testing.T, address string, cfg *tls.Config, id string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		stream, err := pb.NewAgentControlServiceClient(conn).Connect(ctx)
		if err == nil {
			err = stream.Send(&pb.AgentEnvelope{Payload: &pb.AgentEnvelope_Hello{Hello: &pb.AgentHello{ClusterId: id}}})
		}
		if err == nil {
			_, err = stream.Recv()
		}
		if err == nil {
			t.Fatal("invalid TLS identity was accepted")
		}
		t.Log("rejected", err)
	}
	cases := []struct {
		name, id   string
		cert       tls.Certificate
		serverName string
	}{
		{"untrusted-client-CA", "hugofleet-a", foreign.issue(t, "hugofleet-a", "", false), agenttls.ServerName},
		{"wrong-server-SAN", "hugofleet-a", trusted.issue(t, "hugofleet-a", "", false), "wrong.hugofleet.local"},
		{"expired-client", "hugofleet-a", trusted.issue(t, "hugofleet-a", "", true), agenttls.ServerName},
		{"unregistered-identity", "foreign", trusted.issue(t, "foreign", "", false), agenttls.ServerName},
		{"hello-identity-mismatch", "hugofleet-b", trusted.issue(t, "hugofleet-a", "", false), agenttls.ServerName},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			denied(t, addr, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: trusted.pool, ServerName: test.serverName, Certificates: []tls.Certificate{test.cert}}, test.id)
		})
	}
	_, badAddr := startRPC(t, trusted, "wrong.hugofleet.local")
	denied(t, badAddr, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: trusted.pool, ServerName: agenttls.ServerName, Certificates: []tls.Certificate{trusted.issue(t, "hugofleet-a", "", false)}}, "hugofleet-a")
}
func TestDuplicateSessionHeartbeatAndReconnect(t *testing.T) {
	a := ca(t)
	transport, addr := startRPC(t, a, agenttls.ServerName)
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: a.pool, ServerName: agenttls.ServerName, Certificates: []tls.Certificate{a.issue(t, "hugofleet-a", "", false)}}
	first, cancel := connect(t, addr, cfg, "hugofleet-a")
	defer cancel()
	if v, e := first.Recv(); e != nil || v.GetWelcome() == nil {
		t.Fatalf("valid handshake %v %v", v, e)
	}
	second, cancel2 := connect(t, addr, cfg, "hugofleet-a")
	defer cancel2()
	if _, e := second.Recv(); status.Code(e) != codes.AlreadyExists {
		t.Fatalf("duplicate session %v", e)
	}
	if _, e := first.Recv(); status.Code(e) != codes.DeadlineExceeded {
		t.Fatalf("missing heartbeat not rejected %v", e)
	}
	if connected, _, _ := transport.ClusterState("hugofleet-a"); connected {
		t.Fatal("expired cluster still connected")
	}
	third, cancel3 := connect(t, addr, cfg, "hugofleet-a")
	defer cancel3()
	if v, e := third.Recv(); e != nil || v.GetWelcome() == nil {
		t.Fatalf("reconnect %v %v", v, e)
	}
}

type slowWork struct{}

func (slowWork) Apply(context.Context, domain.Application, platform.ApplicationSpec) (string, error) {
	return "uid-1", nil
}
func (slowWork) Delete(context.Context, domain.Application) (domain.Observation, error) {
	return domain.Observation{Deleted: true}, nil
}
func (slowWork) DeletionStatus(context.Context, domain.Application) (domain.Observation, error) {
	return domain.Observation{UID: "uid-1"}, nil
}
func (slowWork) Observe(ctx context.Context, a domain.Application, s platform.ApplicationSpec) (domain.Observation, error) {
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return domain.Observation{}, ctx.Err()
	case <-timer.C:
	}
	return domain.Observation{UID: "uid-1", Ready: true, Applied: true, Generation: 1, ObservedGeneration: 1, ObservedRevision: s.Revision}, nil
}
func TestActualAgentClientMaintainsHeartbeatDuringBoundedCommand(t *testing.T) {
	a := ca(t)
	transport, addr := startRPC(t, a, agenttls.ServerName)
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	worker := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: domain.Namespace, Name: agent.LedgerName}}).Build()
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: a.pool, ServerName: agenttls.ServerName, Certificates: []tls.Certificate{a.issue(t, "hugofleet-a", "", false)}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- (&agent.Client{Endpoint: addr, Cluster: "hugofleet-a", TLS: cfg, Engine: &agent.Engine{Cluster: "hugofleet-a", Ledger: &agent.KubeLedger{Client: worker}, Workloads: slowWork{}}, Heartbeat: 20 * time.Millisecond}).Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Agent failed graceful cancellation")
		}
	})
	deadline := time.Now().Add(2 * time.Second)
	for {
		connected, _, _ := transport.ClusterState("hugofleet-a")
		if connected {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Agent did not connect")
		}
		time.Sleep(10 * time.Millisecond)
	}
	application := domain.Application{ID: "test", TenantID: "demo", Name: "pricing", DesiredRevision: 1}
	spec, _ := domain.DefaultPolicy().Spec(domain.DeployRequest{Image: "pricing-api:v1"})
	c := &pb.HubEnvelope{CommandId: "snapshot-1", ExpiresAt: time.Now().Add(time.Second).UnixMilli(), Payload: &pb.HubEnvelope_Snapshot{Snapshot: &pb.RequestSnapshot{Application: agentprotocol.Identity(application), Spec: agentprotocol.Spec(spec)}}}
	opCtx, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	ob, err := transport.Call(opCtx, "hugofleet-a", c)
	if err != nil || !ob.Ready {
		t.Fatalf("actual Agent bidi command %+v %v", ob, err)
	}
	if connected, _, _ := transport.ClusterState("hugofleet-a"); !connected {
		t.Fatal("heartbeat stopped during command")
	}
}
