package agentprotocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	pb "github.com/win30221/hugofleet/stages/m3/api/agent/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

func Identity(a domain.Application) *pb.ApplicationIdentity {
	return &pb.ApplicationIdentity{ApplicationId: a.ID, TenantId: a.TenantID, Namespace: domain.NamespaceForTenant(a.TenantID), Name: a.Name, Uid: a.KubernetesUID, Revision: a.DesiredRevision}
}
func App(v *pb.ApplicationIdentity, cluster string) domain.Application {
	return domain.Application{ID: v.GetApplicationId(), TenantID: v.GetTenantId(), ClusterID: cluster, Name: v.GetName(), KubernetesUID: v.GetUid(), DesiredRevision: v.GetRevision()}
}
func Spec(s platform.ApplicationSpec) *pb.ApplicationSpec {
	return &pb.ApplicationSpec{Image: s.Image, Replicas: s.Replicas, Port: s.Port, Requests: &pb.ResourceValues{Cpu: s.Resources.Requests.CPU, Memory: s.Resources.Requests.Memory}, Limits: &pb.ResourceValues{Cpu: s.Resources.Limits.CPU, Memory: s.Resources.Limits.Memory}, ReadinessPath: s.ReadinessPath, LivenessPath: s.LivenessPath}
}
func Platform(s *pb.ApplicationSpec, revision int64) platform.ApplicationSpec {
	return platform.ApplicationSpec{Revision: revision, Image: s.GetImage(), Replicas: s.GetReplicas(), Port: s.GetPort(), Resources: platform.ApplicationResources{Requests: platform.ResourceValues{CPU: s.GetRequests().GetCpu(), Memory: s.GetRequests().GetMemory()}, Limits: platform.ResourceValues{CPU: s.GetLimits().GetCpu(), Memory: s.GetLimits().GetMemory()}}, ReadinessPath: s.GetReadinessPath(), LivenessPath: s.GetLivenessPath()}
}
func Status(o domain.Observation) *pb.WorkloadStatus {
	s := &pb.WorkloadStatus{Uid: o.UID, Applied: o.Applied, Ready: o.Ready, Deleted: o.Deleted, ErrorCode: o.ErrorCode, Message: o.Message, ObservedRevision: o.ObservedRevision, Generation: o.Generation, ObservedGeneration: o.ObservedGeneration}
	for _, c := range o.Conditions {
		ms := int64(0)
		if !c.LastTransitionTime.IsZero() {
			ms = c.LastTransitionTime.UnixMilli()
		}
		s.Conditions = append(s.Conditions, &pb.Condition{Type: c.Type, Status: string(c.Status), Reason: c.Reason, Message: c.Message, ObservedGeneration: c.ObservedGeneration, LastTransitionAt: ms})
	}
	return s
}
func Observation(o *pb.WorkloadStatus) domain.Observation {
	v := domain.Observation{UID: o.GetUid(), Applied: o.GetApplied(), Ready: o.GetReady(), Deleted: o.GetDeleted(), ErrorCode: o.GetErrorCode(), Message: o.GetMessage(), ObservedRevision: o.GetObservedRevision(), Generation: o.GetGeneration(), ObservedGeneration: o.GetObservedGeneration()}
	for _, c := range o.GetConditions() {
		cond := metav1.Condition{Type: c.GetType(), Status: metav1.ConditionStatus(c.GetStatus()), Reason: c.GetReason(), Message: c.GetMessage(), ObservedGeneration: c.GetObservedGeneration()}
		if c.GetLastTransitionAt() > 0 {
			cond.LastTransitionTime = metav1.NewTime(time.UnixMilli(c.GetLastTransitionAt()))
		}
		v.Conditions = append(v.Conditions, cond)
	}
	return v
}
func Hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Hash only the typed action; excludes transient correlation/deadline fields.
func PayloadHash(c *pb.HubEnvelope) string {
	switch {
	case c.GetApply() != nil:
		return Hash(c.GetApply())
	case c.GetDelete() != nil:
		return Hash(c.GetDelete())
	case c.GetSnapshot() != nil:
		return Hash(c.GetSnapshot())
	case c.GetTenantProfile() != nil:
		return Hash(c.GetTenantProfile())
	default:
		return ""
	}
}

func Profile(p domain.TenantProfile) *pb.TenantProfile {
	return &pb.TenantProfile{TenantId: p.TenantID, ClusterId: p.ClusterID, Namespace: p.Namespace, BootstrapUid: p.BootstrapUID, Hard: p.Hard, Defaults: p.Defaults}
}
func TenantProfile(p *pb.TenantProfile) domain.TenantProfile {
	return domain.TenantProfile{TenantID: p.GetTenantId(), ClusterID: p.GetClusterId(), Namespace: p.GetNamespace(), BootstrapUID: p.GetBootstrapUid(), Hard: p.GetHard(), Defaults: p.GetDefaults()}
}
