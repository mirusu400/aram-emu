//go:build !windows || !amd64

package systemintegration

import "github.com/mirusu400/aram-core/application"

// Other hosts retain the core registry's measured preference until a
// whole-phone benchmark establishes a more specific winner for that target.
const fastestSystemCPUBackend = application.FastestBackend
