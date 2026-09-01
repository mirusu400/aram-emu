package systemintegration

import "github.com/mirusu400/aram-core/application"

// concreteSystemCPUBackend resolves the portable "fastest" preference for a
// whole-phone workload. Explicit core choices always pass through unchanged.
func concreteSystemCPUBackend(name string) string {
	if name == application.FastestBackend {
		return fastestSystemCPUBackend
	}
	return name
}
