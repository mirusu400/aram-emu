package integration

import (
	"testing"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-frontend/frontend"
)

func TestAudioChannelsReachApplicationFactory(t *testing.T) {
	backend := NewBackend(nil)
	if err := backend.ConfigureAudio(frontend.AudioSettings{OutputChannels: 2}); err != nil {
		t.Fatal(err)
	}
	factory, ok := backend.factoryForCreate().(application.Factory)
	if !ok {
		t.Fatal("default factory is not application.Factory")
	}
	if factory.OutputChannels != 2 {
		t.Fatalf("factory output channels = %d, want stereo", factory.OutputChannels)
	}
	if err := backend.ConfigureAudio(frontend.AudioSettings{OutputChannels: 3}); err == nil {
		t.Fatal("invalid output channels were accepted")
	}
}
