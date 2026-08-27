package integration

import (
	"time"

	"github.com/mirusu400/aram-frontend/frontend"
)

// coreHapticsSource is the optional aram-core contract for reading the guest's
// current vibration request: motor strength (0-100) and the time remaining
// before it stops.
type coreHapticsSource interface {
	Vibration() (uint8, time.Duration)
}

// Haptics reports the guest's vibration so the frontend can actuate a real
// gamepad rumble motor or phone vibrator. A core that does not expose vibration
// yields the zero state, so the frontend drives no haptics.
func (backend *Backend) Haptics() frontend.HapticsState {
	machine := backend.currentMachine()
	if machine == nil {
		return frontend.HapticsState{}
	}
	source, ok := unwrapMachine(machine).(coreHapticsSource)
	if !ok {
		return frontend.HapticsState{}
	}
	level, remaining := source.Vibration()
	return frontend.HapticsState{Level: level, Duration: remaining}
}
