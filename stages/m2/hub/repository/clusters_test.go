package repository

import (
	"context"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"testing"
	"time"
)

func TestRemoteRegistryIsolatedConnectivityAndMode(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	if err := s.ConfigureRemote(ctx, []string{"hugofleet-a", "hugofleet-b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckLocal(ctx); err == nil {
		t.Fatal("M2 opened M3 registry")
	}
	if err := s.ClusterConnectivity(ctx, "hugofleet-a", true, 100, "m3"); err != nil {
		t.Fatal(err)
	}
	c := intent("a")
	c.ClusterID = "hugofleet-a"
	if _, err := s.Accept(ctx, c, time.Minute); err != nil {
		t.Fatal(err)
	}
	c = intent("b")
	c.ClusterID = "hugofleet-b"
	if _, err := s.Accept(ctx, c, time.Minute); err == nil {
		t.Fatal("offline B accepted")
	}
	clusters, err := s.Clusters(ctx)
	if err != nil || len(clusters) != 2 || clusters[0].LastSeenAt != 100 || clusters[1].State != "DISCONNECTED" {
		t.Fatalf("bad registry %+v %v", clusters, err)
	}
	c = intent("foreign")
	c.ClusterID = "foreign"
	if _, err := s.Accept(ctx, c, time.Minute); err == nil {
		t.Fatal("unknown cluster accepted")
	}
	s2, _ := testStore(t)
	accept(t, s2, intent("m2"))
	if err := s2.ConfigureRemote(ctx, []string{"hugofleet-a", "hugofleet-b"}); err == nil {
		t.Fatal("M3 migrated M2 application")
	}
	_ = domain.ClusterID
}
