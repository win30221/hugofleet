package domain

import (
	"fmt"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const TenantID = "demo"
const ClusterID = "local-kind"
const Namespace = "hf-demo"

type Error struct{ Code, Message string }

func (e *Error) Error() string           { return e.Message }
func Problem(code, message string) error { return &Error{code, message} }

type Tenant struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedAt int64  `json:"created_at"`
}
type Cluster struct {
	AgentVersion string `json:"agent_version,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	State        string `json:"state"`
	LastSeenAt   int64  `json:"last_seen_at"`
	RegisteredAt int64  `json:"registered_at"`
}
type Application struct {
	Observed                Observation `json:"observed"`
	ID                      string      `json:"id"`
	TenantID                string      `json:"tenant_id"`
	ClusterID               string      `json:"cluster_id"`
	Name                    string      `json:"name"`
	DesiredRevision         int64       `json:"desired_revision"`
	AppliedRevision         int64       `json:"applied_revision"`
	LastSuccessfulRevision  int64       `json:"last_successful_revision"`
	Status                  string      `json:"status"`
	LastObservedAt          int64       `json:"last_observed_at"`
	KubernetesUID           string      `json:"kubernetes_uid"`
	ObservationErrorCode    string      `json:"observation_error_code,omitempty"`
	ObservationErrorMessage string      `json:"observation_error_message,omitempty"`
}
type Release struct {
	ID                     string                   `json:"id"`
	AppID                  string                   `json:"app_id"`
	Revision               int64                    `json:"revision"`
	Spec                   platform.ApplicationSpec `json:"spec_snapshot"`
	State                  string                   `json:"state"`
	CreatedBy              string                   `json:"created_by"`
	CreatedAt              int64                    `json:"created_at"`
	RollbackSourceRevision int64                    `json:"rollback_source_revision,omitempty"`
}
type Operation struct {
	Traceparent  string `json:"traceparent,omitempty"`
	Tracestate   string `json:"tracestate,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
	ID           string `json:"id"`
	AppID        string `json:"app_id"`
	ReleaseID    string `json:"release_id,omitempty"`
	CommandID    string `json:"command_id"`
	Type         string `json:"type"`
	State        string `json:"state"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	DeadlineAt   int64  `json:"deadline_at"`
	FinishedAt   int64  `json:"finished_at,omitempty"`
}

func (o Operation) Active() bool {
	return o.State == "QUEUED" || o.State == "DISPATCHED" || o.State == "RUNNING"
}

type Event struct {
	ID          string `json:"id"`
	OperationID string `json:"operation_id"`
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	OccurredAt  int64  `json:"occurred_at"`
}
type Detail struct {
	Application Application `json:"application"`
	Releases    []Release   `json:"releases"`
	Operations  []Operation `json:"operations"`
	Events      []Event     `json:"events"`
}
type Accepted struct {
	ApplicationID string `json:"application_id"`
	OperationID   string `json:"operation_id"`
	Revision      int64  `json:"revision"`
	State         string `json:"state"`
}

type Page[T any] struct {
	Items     []T    `json:"items"`
	NextAfter string `json:"next_after"`
}
type DeployRequest struct {
	Name                    string                        `json:"name,omitempty"`
	ClusterID               string                        `json:"cluster_id,omitempty"`
	ExpectedDesiredRevision *int64                        `json:"expected_desired_revision"`
	Image                   string                        `json:"image"`
	Replicas                int32                         `json:"replicas,omitempty"`
	Port                    int32                         `json:"port,omitempty"`
	Resources               platform.ApplicationResources `json:"resources"`
	ReadinessPath           string                        `json:"readiness_path,omitempty"`
	LivenessPath            string                        `json:"liveness_path,omitempty"`
}
type RollbackRequest struct {
	TargetRevision          int64  `json:"target_revision"`
	ExpectedDesiredRevision *int64 `json:"expected_desired_revision"`
}
type DeleteRequest struct {
	ExpectedDesiredRevision *int64 `json:"expected_desired_revision"`
}
type Policy struct {
	DefaultResources                         platform.ApplicationResources
	MaxCPU, MaxMemory, SurgeCPU, SurgeMemory string
}

func DefaultPolicy() Policy {
	return Policy{platform.ApplicationResources{Requests: platform.ResourceValues{CPU: "100m", Memory: "64Mi"}, Limits: platform.ResourceValues{CPU: "300m", Memory: "128Mi"}}, "1", "512Mi", "2", "1Gi"}
}
func (p Policy) Validate() error {
	s := platform.ApplicationSpec{Revision: 1, Image: "pricing-api:v1", Replicas: 1, Port: 8080, Resources: p.DefaultResources, ReadinessPath: "/readyz", LivenessPath: "/healthz"}
	if err := s.Validate(); err != nil {
		return err
	}
	for _, v := range []string{p.MaxCPU, p.MaxMemory, p.SurgeCPU, p.SurgeMemory} {
		q, e := resource.ParseQuantity(v)
		if e != nil || q.Sign() <= 0 {
			return fmt.Errorf("invalid positive resource policy quantity")
		}
	}
	_, err := p.Spec(DeployRequest{Image: s.Image})
	return err
}
func (p Policy) Spec(r DeployRequest) (platform.ApplicationSpec, error) {
	s := platform.ApplicationSpec{Revision: 1, Image: r.Image, Replicas: r.Replicas, Port: r.Port, Resources: r.Resources, ReadinessPath: r.ReadinessPath, LivenessPath: r.LivenessPath}
	if s.Replicas == 0 {
		s.Replicas = 1
	}
	if s.Port == 0 {
		s.Port = 8080
	}
	if s.ReadinessPath == "" {
		s.ReadinessPath = "/readyz"
	}
	if s.LivenessPath == "" {
		s.LivenessPath = "/healthz"
	}
	for _, pair := range [][2]*string{{&s.Resources.Requests.CPU, &p.DefaultResources.Requests.CPU}, {&s.Resources.Requests.Memory, &p.DefaultResources.Requests.Memory}, {&s.Resources.Limits.CPU, &p.DefaultResources.Limits.CPU}, {&s.Resources.Limits.Memory, &p.DefaultResources.Limits.Memory}} {
		if *pair[0] == "" {
			*pair[0] = *pair[1]
		}
	}
	if err := s.Validate(); err != nil {
		return s, Problem("INVALID_ARGUMENT", err.Error())
	}
	for _, v := range [][2]string{{s.Resources.Limits.CPU, p.MaxCPU}, {s.Resources.Limits.Memory, p.MaxMemory}} {
		q := resource.MustParse(v[0])
		if q.Cmp(resource.MustParse(v[1])) > 0 {
			return s, Problem("QUOTA_EXCEEDED", "per-Pod resource maximum exceeded")
		}
	}
	for _, v := range [][2]string{{s.Resources.Requests.CPU, p.SurgeCPU}, {s.Resources.Requests.Memory, p.SurgeMemory}} {
		q := resource.MustParse(v[0])
		q.Mul(int64(s.Replicas + 1))
		if q.Cmp(resource.MustParse(v[1])) > 0 {
			return s, Problem("QUOTA_EXCEEDED", "requests including one rolling-update surge Pod exceed development policy")
		}
	}
	return s, nil
}
func ValidateName(name string) error {
	if len(validation.IsDNS1123Label(name)) != 0 {
		return Problem("INVALID_ARGUMENT", "name must be a DNS label of at most 63 characters")
	}
	return nil
}

type Observation struct {
	ObservedRevision   int64              `json:"observed_revision"`
	Generation         int64              `json:"generation"`
	ObservedGeneration int64              `json:"observed_generation"`
	Conditions         []metav1.Condition `json:"conditions"`
	UID                string             `json:"uid"`
	// Applied confirms that the bound CR has the exact accepted spec. It does
	// not imply readiness or authorize changing a terminal operation.
	Applied   bool   `json:"applied"`
	Ready     bool   `json:"ready"`
	Deleted   bool   `json:"deleted"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"message,omitempty"`
}
type Change struct {
	TenantID, AppID, Name, ClusterID, Key, Hash, Kind string
	Expected                                          int64
	SourceRevision                                    int64
	Spec                                              platform.ApplicationSpec
	// Pure validation runs after deduplication, inside the acceptance transaction.
	ValidateSpec func(platform.ApplicationSpec) error
}
