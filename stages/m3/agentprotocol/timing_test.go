package agentprotocol

import (
	"testing"
	"time"
)

func TestTimingBounds(t *testing.T) {
	for _, v := range []struct {
		h, min, max time.Duration
		valid       bool
	}{
		{5 * time.Second, time.Second, 30 * time.Second, true},
		{time.Second, 100 * time.Millisecond, 5 * time.Minute, true},
		{0, time.Second, 30 * time.Second, false},
		{31 * time.Second, time.Second, 30 * time.Second, false},
		{time.Second, 99 * time.Millisecond, time.Second, false},
		{time.Second, 2 * time.Second, time.Second, false},
		{time.Second, time.Second, 6 * time.Minute, false},
	} {
		if err := ValidateAgentTiming(v.h, v.min, v.max); (err == nil) != v.valid {
			t.Fatalf("%+v: %v", v, err)
		}
	}
	for _, v := range []struct {
		value time.Duration
		valid bool
	}{{3 * time.Second, true}, {5 * time.Minute, true}, {0, false}, {2 * time.Second, false}, {6 * time.Minute, false}} {
		if err := ValidateHeartbeatTimeout(v.value); (err == nil) != v.valid {
			t.Fatalf("%+v: %v", v, err)
		}
	}
}
