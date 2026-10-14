package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/win30221/hugofleet/stages/m3/agentprotocol"
	pb "github.com/win30221/hugofleet/stages/m3/api/agent/v1"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"log/slog"
	"math/rand/v2"
	"time"
)

type Client struct {
	Connection                 func(bool)
	Version                    string
	Endpoint, Cluster          string
	TLS                        *tls.Config
	Engine                     *Engine
	Heartbeat                  time.Duration
	ReconnectMin, ReconnectMax time.Duration
}

func (c *Client) Run(ctx context.Context) error {
	if c.Heartbeat == 0 {
		c.Heartbeat = 5 * time.Second
	}
	if c.ReconnectMin == 0 {
		c.ReconnectMin = time.Second
	}
	if c.ReconnectMax == 0 {
		c.ReconnectMax = 30 * time.Second
	}
	// CLI enforces production heartbeat bounds; transport tests may use a shorter
	// positive interval to exercise expiry without replacing the real TLS stream.
	if c.Heartbeat <= 0 {
		return fmt.Errorf("heartbeat must be positive")
	}
	if err := agentprotocol.ValidateAgentTiming(5*time.Second, c.ReconnectMin, c.ReconnectMax); err != nil {
		return err
	}
	delay := c.ReconnectMin
	for ctx.Err() == nil {
		started := time.Now()
		err := c.connect(ctx)
		if ctx.Err() != nil {
			break
		}
		slog.Warn("Agent channel closed; reconnecting", "cluster_id", c.Cluster, "error", err)
		if time.Since(started) > 30*time.Second {
			delay = c.ReconnectMin
		}
		timer := time.NewTimer(delay/2 + time.Duration(rand.Int64N(int64(delay/2))))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		delay = min(delay*2, c.ReconnectMax)
	}
	return nil
}
func (c *Client) connect(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	conn, err := grpc.NewClient(c.Endpoint, grpc.WithTransportCredentials(credentials.NewTLS(c.TLS)), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(128<<10), grpc.MaxCallSendMsgSize(128<<10)))
	if err != nil {
		return err
	}
	defer conn.Close()
	stream, err := pb.NewAgentControlServiceClient(conn).Connect(ctx)
	if err != nil {
		return err
	}
	version := c.Version
	if version == "" {
		version = "m3"
	}
	capabilities := []string{"ApplyApplication", "DeleteApplication", "RequestSnapshot"}
	if c.Engine.Profiles != nil {
		capabilities = append(capabilities, "RequestTenantProfile")
	}
	if err = stream.Send(&pb.AgentEnvelope{Payload: &pb.AgentEnvelope_Hello{Hello: &pb.AgentHello{ClusterId: c.Cluster, AgentVersion: version, Capabilities: capabilities}}}); err != nil {
		return err
	}
	incoming := make(chan *pb.HubEnvelope, 1)
	failed := make(chan error, 1)
	go func() {
		for {
			v, e := stream.Recv()
			if e != nil {
				failed <- e
				return
			}
			select {
			case incoming <- v:
			case <-ctx.Done():
				return
			}
		}
	}()
	handshake := time.NewTimer(5 * time.Second)
	channelID := ""
	defer handshake.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-failed:
		return err
	case <-handshake.C:
		return fmt.Errorf("Hub handshake timeout")
	case welcome := <-incoming:
		if welcome.GetWelcome() == nil {
			return fmt.Errorf("missing Hub welcome")
		}
		channelID = welcome.GetWelcome().SessionId
	}
	ctx = observability.WithChannel(ctx, channelID)
	if c.Connection != nil {
		c.Connection(true)
		defer c.Connection(false)
	}
	slog.Info("Agent mTLS channel established", "cluster_id", c.Cluster, "channel_id", observability.ChannelContext(ctx))
	ticker := time.NewTicker(c.Heartbeat)
	defer ticker.Stop()
	// A single executor and one pending input bound command concurrency and memory.
	var done chan *pb.CommandResult
	var input <-chan *pb.HubEnvelope = incoming
	for {
		select {
		case <-ctx.Done():
			cancel()
			if done != nil {
				<-done
			}
			return ctx.Err()
		case err = <-failed:
			cancel()
			if done != nil {
				<-done
			}
			return err
		case command := <-input:
			done = make(chan *pb.CommandResult, 1)
			input = nil
			output := done
			go func() { output <- c.Engine.Execute(ctx, command) }()
		case result := <-done:
			done = nil
			input = incoming
			if err = stream.Send(&pb.AgentEnvelope{Payload: &pb.AgentEnvelope_Result{Result: result}}); err != nil {
				return err
			}
		case <-ticker.C:
			if err = stream.Send(&pb.AgentEnvelope{Payload: &pb.AgentEnvelope_Heartbeat{Heartbeat: &pb.Heartbeat{Timestamp: time.Now().UnixMilli()}}}); err != nil {
				cancel()
				if done != nil {
					<-done
				}
				return err
			}
		}
	}
}
