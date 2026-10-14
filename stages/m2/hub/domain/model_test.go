package domain

import (
	"testing"
)

func TestDefaultsAndSurgePolicy(t *testing.T) {
	p := DefaultPolicy()
	if e := p.Validate(); e != nil {
		t.Fatal(e)
	}
	s, e := p.Spec(DeployRequest{Image: "pricing-api:v1"})
	if e != nil || s.Replicas != 1 || s.Resources != p.DefaultResources || s.Revision != 1 {
		t.Fatalf("bad defaults %+v %v", s, e)
	}
	p.SurgeCPU = "300m"
	_, e = p.Spec(DeployRequest{Image: "pricing-api:v1", Replicas: 3})
	if e == nil {
		t.Fatal("rolling surge was not included")
	}
	_, e = p.Spec(DeployRequest{Image: "pricing-api:v1", ReadinessPath: "//host"})
	if e == nil {
		t.Fatal("host probe accepted")
	}
}
