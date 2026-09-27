// Package productconfig shares portable composition defaults between product
// adapters and headless measurement hosts. It has no frontend dependency.
package productconfig

import (
	authd "github.com/mirusu400/aram-authd"
	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-emu/carrier"
)

func ApplicationFactory() application.Factory {
	factory := application.NewFactory()
	// The product opts into structurally validated, not cryptographically
	// verified BREW modules. Library consumers remain opted out.
	factory.AllowUntrustedBREW = true
	factory.FrameRunBudget = application.DefaultHandsetRunBudget
	factory.RaptorFrameRunBudget = application.DefaultRaptorFrameRunBudget
	factory.KTFRunBudget = application.DefaultKTFHandsetRunBudget
	factory.RaptorNet = carrier.AuthdRaptorNet(authd.Grant{})
	return factory
}
