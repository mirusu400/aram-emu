package core

import "testing"

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateEmpty, "empty"},
		{StateReady, "ready"},
		{StateRunning, "running"},
		{StatePaused, "paused"},
		{StateStopped, "stopped"},
		{StateFaulted, "faulted"},
		{State(255), "unknown"},
	}
	for _, test := range tests {
		if got := test.state.String(); got != test.want {
			t.Fatalf("State(%d).String() = %q, want %q", test.state, got, test.want)
		}
	}
}
