package v1alpha1

import (
	"strings"
	"testing"
)

func validSpec() ApplicationSpec {
	return ApplicationSpec{Revision: 1, Image: "pricing-api:v1", Replicas: 1, Port: 8080, ReadinessPath: "/readyz", LivenessPath: "/healthz",
		Resources: ApplicationResources{Requests: ResourceValues{CPU: "100m", Memory: "64Mi"}, Limits: ResourceValues{CPU: "300m", Memory: "128Mi"}}}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ApplicationSpec)
		valid  bool
	}{
		{"base", func(s *ApplicationSpec) {}, true},
		{"registry digest", func(s *ApplicationSpec) { s.Image = "localhost:5000/team/pricing@sha256:" + strings.Repeat("a", 64) }, true},
		{"equal limits", func(s *ApplicationSpec) { s.Resources.Limits = s.Resources.Requests }, true},
		{"max replicas", func(s *ApplicationSpec) { s.Replicas = 3 }, true},
		{"bad revision", func(s *ApplicationSpec) { s.Revision = 0 }, false},
		{"zero replicas", func(s *ApplicationSpec) { s.Replicas = 0 }, false},
		{"excess replicas", func(s *ApplicationSpec) { s.Replicas = 4 }, false},
		{"bad port", func(s *ApplicationSpec) { s.Port = 65536 }, false},
		{"empty image", func(s *ApplicationSpec) { s.Image = "" }, false},
		{"image spaces", func(s *ApplicationSpec) { s.Image = "pricing api:v1" }, false},
		{"image url", func(s *ApplicationSpec) { s.Image = "https://example.com/app" }, false},
		{"uppercase repository", func(s *ApplicationSpec) { s.Image = "Pricing:v1" }, false},
		{"malformed digest", func(s *ApplicationSpec) { s.Image = "app@sha256:abc" }, false},
		{"probe url", func(s *ApplicationSpec) { s.ReadinessPath = "https://example.com/readyz" }, false},
		{"probe host", func(s *ApplicationSpec) { s.LivenessPath = "//example.com/healthz" }, false},
		{"probe query", func(s *ApplicationSpec) { s.ReadinessPath = "/readyz?x=1" }, false},
		{"quantity malformed", func(s *ApplicationSpec) { s.Resources.Requests.CPU = "100watts" }, false},
		{"quantity zero", func(s *ApplicationSpec) { s.Resources.Requests.Memory = "0" }, false},
		{"quantity negative", func(s *ApplicationSpec) { s.Resources.Limits.CPU = "-1" }, false},
		{"cpu limit low", func(s *ApplicationSpec) { s.Resources.Limits.CPU = "50m" }, false},
		{"memory limit low", func(s *ApplicationSpec) { s.Resources.Limits.Memory = "32Mi" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSpec()
			tc.mutate(&s)
			err := s.Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
