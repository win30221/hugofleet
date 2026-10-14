package v1alpha1

import (
	_ "crypto/sha256" // Register the only digest algorithm accepted by the CRD policy.
	"fmt"
	"regexp"
	"strings"

	"github.com/distribution/reference"
	"k8s.io/apimachinery/pkg/api/resource"
)

var pathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*$`)
var imagePattern = regexp.MustCompile(`^([a-z0-9]+([.-][a-z0-9]+)*(:[0-9]{1,5})?/)?[a-z0-9]+(([._]|__|-+)[a-z0-9]+)*(/[a-z0-9]+(([._]|__|-+)[a-z0-9]+)*)*(:[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(@sha256:[a-f0-9]{64})?$`)

func (s ApplicationSpec) Validate() error {
	if s.Revision < 1 {
		return fmt.Errorf("revision must be positive")
	}
	if len(s.Image) == 0 || len(s.Image) > 512 || strings.TrimSpace(s.Image) != s.Image {
		return fmt.Errorf("image is empty, too long or contains whitespace")
	}
	if !imagePattern.MatchString(s.Image) {
		return fmt.Errorf("image does not match the supported repository/tag/sha256 policy")
	}
	if _, err := reference.ParseNormalizedNamed(s.Image); err != nil {
		return fmt.Errorf("invalid image reference: %w", err)
	}
	if s.Replicas < 1 || s.Replicas > 3 {
		return fmt.Errorf("replicas must be from 1 to 3")
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("port must be from 1 to 65535")
	}
	for _, p := range []string{s.ReadinessPath, s.LivenessPath} {
		if len(p) > 256 || !pathPattern.MatchString(p) || strings.HasPrefix(p, "//") {
			return fmt.Errorf("probe must be a local absolute path without query or host")
		}
	}
	for _, pair := range [][2]string{{s.Resources.Requests.CPU, s.Resources.Limits.CPU}, {s.Resources.Requests.Memory, s.Resources.Limits.Memory}} {
		request, err := resource.ParseQuantity(pair[0])
		if err != nil || len(pair[0]) > 32 || request.Sign() <= 0 {
			return fmt.Errorf("resource request must be a positive Kubernetes quantity")
		}
		limit, err := resource.ParseQuantity(pair[1])
		if err != nil || len(pair[1]) > 32 || limit.Sign() <= 0 {
			return fmt.Errorf("resource limit must be a positive Kubernetes quantity")
		}
		if limit.Cmp(request) < 0 {
			return fmt.Errorf("resource limit must be at least the request")
		}
	}
	return nil
}
