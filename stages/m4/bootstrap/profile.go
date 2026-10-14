package bootstrap

import "github.com/win30221/hugofleet/stages/m3/agentprofile"

// Shared names belong to the read-only Agent protocol. Administrator provisioning
// remains exclusively in this M4 package and is not imported by the Agent binary.
const (
	ReceiptName      = agentprofile.ReceiptName
	QuotaName        = agentprofile.QuotaName
	LimitsName       = agentprofile.LimitsName
	ControlNamespace = agentprofile.ControlNamespace
	ManagedLabel     = agentprofile.ManagedLabel
	TenantLabel      = agentprofile.TenantLabel
	ClusterLabel     = agentprofile.ClusterLabel
	Manager          = agentprofile.Manager
)

type Reader = agentprofile.Reader
