package hostbackend

import "testing"

type textInputRecordingAdapter struct {
	*recordingAdapter
	active bool
}

func (adapter *textInputRecordingAdapter) TextInputActive() bool {
	return adapter.active
}

func TestTextInputActiveFollowsSelectedAdapter(t *testing.T) {
	application := &textInputRecordingAdapter{
		recordingAdapter: &recordingAdapter{name: "application"},
		active:           true,
	}
	system := &textInputRecordingAdapter{
		recordingAdapter: &recordingAdapter{name: "system"},
	}
	backend := newBackend(application, system)
	if !backend.TextInputActive() {
		t.Fatal("application text input hint was not forwarded")
	}

	backend.active = system
	if backend.TextInputActive() {
		t.Fatal("outgoing application text input leaked into system mode")
	}
}
