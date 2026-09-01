//go:build windows && amd64

package systemintegration

import (
	"testing"

	"github.com/mirusu400/aram-core/application"
)

func TestFastestSystemCPUUsesMeasuredWindowsWinner(t *testing.T) {
	if got := concreteSystemCPUBackend(application.FastestBackend); got != "jit" {
		t.Fatalf("fastest Windows system CPU = %q, want %q", got, "jit")
	}
	if got := concreteSystemCPUBackend("native"); got != "native" {
		t.Fatalf("explicit Windows system CPU = %q, want %q", got, "native")
	}
}
