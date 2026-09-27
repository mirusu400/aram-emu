package productconfig

import (
	"github.com/mirusu400/aram-core/application"
	"testing"
)

func TestProductFactoryKeepsIndependentRuntimeBudgets(t *testing.T) {
	factory := ApplicationFactory()
	if factory.FrameRunBudget != application.DefaultHandsetRunBudget || factory.KTFRunBudget != application.DefaultKTFHandsetRunBudget || factory.RaptorFrameRunBudget != application.DefaultRaptorFrameRunBudget || !factory.AllowUntrustedBREW || factory.RaptorNet == nil {
		t.Fatalf("product defaults changed: %+v", factory)
	}
}
