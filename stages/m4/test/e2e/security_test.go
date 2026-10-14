//go:build e2e

package e2e

import "testing"

func TestRealTenantSecurity(t *testing.T) { RunRealTenantSecurity(t, "m4") }
