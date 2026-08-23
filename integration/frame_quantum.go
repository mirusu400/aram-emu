package integration

import "time"

// defaultFrameQuantum matches the native-WIPI presentation quantum in
// aram-core. It is only used when the loaded machine cannot report its own.
const defaultFrameQuantum = 16 * time.Millisecond

// coreFrameQuantumReporter is the optional aram-core contract for discovering
// how much guest time one StepFrame advances.
type coreFrameQuantumReporter interface {
	FrameQuantum() time.Duration
}

// FrameQuantum reports how much guest time one RunFrame advances. The core's
// virtual clock never reads host wall time, so a driver has to issue quanta at
// this rate to run a title at handset speed, and the rate is not the same for
// every runtime.
func (backend *Backend) FrameQuantum() time.Duration {
	// The published machine is the cheat wrapper, which forwards the machine
	// contract but not the optional reporting interfaces, so the quantum has to
	// be read from the core machine underneath it. Reading it through the
	// wrapper silently returned the native-WIPI fallback for every title and
	// paced KTF titles four percent fast.
	machine := unwrapMachine(backend.currentMachine())
	if machine == nil {
		return defaultFrameQuantum
	}
	reporter, ok := machine.(coreFrameQuantumReporter)
	if !ok {
		return defaultFrameQuantum
	}
	quantum := reporter.FrameQuantum()
	if quantum <= 0 {
		return defaultFrameQuantum
	}
	return quantum
}
