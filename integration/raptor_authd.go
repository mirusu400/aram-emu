package integration

import (
	authd "github.com/mirusu400/aram-authd"
	"github.com/mirusu400/aram-core/netauth"
	"github.com/mirusu400/aram-emu/carrier"
)

// AuthdRaptorNet remains the integration package compatibility entry point.
// The implementation is shared with headless product hosts such as libretro.
func AuthdRaptorNet(backend authd.NetBackend) netauth.Backend {
	return carrier.AuthdRaptorNet(backend)
}
