package agentprotocol

import (
	"reflect"
	"testing"
	"time"

	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestObservationRoundTripKeepsWorkerEvidence(t *testing.T) {
	want := domain.Observation{ObservedRevision: 4, Generation: 9, ObservedGeneration: 9, UID: "worker-cr-uid", Applied: true, Ready: true, Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, ObservedGeneration: 9, Reason: "RolloutComplete", Message: "all updated replicas ready", LastTransitionTime: metav1.NewTime(time.UnixMilli(1234567890000))}}}
	if got := Observation(Status(want)); !reflect.DeepEqual(got, want) {
		t.Fatalf("Worker evidence lost: %+v", got)
	}
}
