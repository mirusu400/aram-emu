package mobile

import (
	"math"
	"testing"

	"github.com/mirusu400/aram-frontend/frontend"
)

func TestHostEventWithWordsChecksMobileIntegerBoundary(t *testing.T) {
	event, err := hostEventWithWords(
		frontend.HostEventSMSReceive,
		1,
		2,
		3,
		int64(math.MaxUint32),
	)
	if err != nil {
		t.Fatal(err)
	}
	if event.Words != ([4]uint32{1, 2, 3, math.MaxUint32}) {
		t.Fatalf("words = %#v", event.Words)
	}
	if _, err := hostEventWithWords(frontend.HostEventSerialReceive, -1, 0, 0, 0); err == nil {
		t.Fatal("negative mobile word was accepted")
	}
	if _, err := hostEventWithWords(frontend.HostEventSerialReceive, int64(math.MaxUint32)+1, 0, 0, 0); err == nil {
		t.Fatal("oversized mobile word was accepted")
	}
}
