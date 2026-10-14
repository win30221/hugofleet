package agentprotocol

import (
	"fmt"
	"time"
)

func ValidateAgentTiming(heartbeat, retryMin, retryMax time.Duration) error {
	if heartbeat < time.Second || heartbeat > 30*time.Second || retryMin < 100*time.Millisecond || retryMax > 5*time.Minute || retryMin > retryMax {
		return fmt.Errorf("heartbeat must be 1s..30s; reconnect must satisfy 100ms <= min <= max <= 5m")
	}
	return nil
}

func ValidateHeartbeatTimeout(timeout time.Duration) error {
	if timeout < 3*time.Second || timeout > 5*time.Minute {
		return fmt.Errorf("heartbeat timeout must be 3s..5m and should exceed the Agent heartbeat interval")
	}
	return nil
}
