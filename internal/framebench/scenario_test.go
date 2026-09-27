package framebench

import (
	"image"
	"strings"
	"testing"
	"time"
)

func testScenario() Scenario {
	return Scenario{Schema: 1, ID: "synthetic", SHA256: strings.Repeat("a", 64), Frames: 4, WarmupFrames: 1, SegmentFrames: 2, Events: []Event{{2, "ok", true}, {3, "ok", false}}}
}

func TestScenarioRejectsInvalidAndUnboundedReplay(t *testing.T) {
	for _, change := range []func(*Scenario){
		func(s *Scenario) { s.SHA256 = "" }, func(s *Scenario) { s.ID = "C:/private" },
		func(s *Scenario) { s.Frames = 1_000_001 }, func(s *Scenario) { s.WarmupFrames = s.Frames },
		func(s *Scenario) { s.Events[1].Frame = 1 }, func(s *Scenario) { s.Events = s.Events[:1] },
		func(s *Scenario) { s.Events[0].Pressed = false }, func(s *Scenario) { s.Events[0].Frame = 0 },
		func(s *Scenario) { s.Checkpoints = []Checkpoint{{2, "wrong"}} },
	} {
		s := testScenario()
		change(&s)
		if s.Validate() == nil {
			t.Fatalf("accepted invalid scenario: %+v", s)
		}
	}
	if _, err := ReadScenario(strings.NewReader(`{"schema":1,"unknown":true}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := ReadScenario(strings.NewReader(strings.Repeat(" ", MaxScenarioBytes+1))); err == nil {
		t.Fatal("oversized scenario accepted")
	}
}

func TestReplayUsesGuestFramesNotWallTime(t *testing.T) {
	for _, delay := range []time.Duration{time.Millisecond, 100 * time.Millisecond} {
		scenario := testScenario()
		session, err := New(scenario, "synthetic", "jit", 16*time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Unix(0, 0)
		var frames []uint64
		for frame := uint64(1); frame <= scenario.Frames; frame++ {
			if err := session.BeforeFrame(now, func(event Event) error { frames = append(frames, frame); return nil }); err != nil {
				t.Fatal(err)
			}
			now = now.Add(delay)
			if err := session.AfterFrame(now, time.Millisecond, nil); err != nil {
				t.Fatal(err)
			}
		}
		if len(frames) != 2 || frames[0] != 2 || frames[1] != 3 || session.Result.Status != "complete" {
			t.Fatalf("incorrect replay: %v %+v", frames, session.Result)
		}
		if session.Result.Measurement.Frames != 3 || session.Result.Measurement.GuestMS != 48 {
			t.Fatalf("warmup polluted measurement: %+v", session.Result.Measurement)
		}
		if session.Result.Measurement.WallMS != float64(3*delay)/float64(time.Millisecond) {
			t.Fatal("wall duration incorrect")
		}
		if len(session.Result.Segments) != 2 {
			t.Fatal("segments missing")
		}
	}
}

func TestCheckpointMismatchCannotPass(t *testing.T) {
	scenario := testScenario()
	scenario.Checkpoints = []Checkpoint{{1, strings.Repeat("b", 64)}}
	session, _ := New(scenario, "synthetic", "jit", time.Millisecond)
	now := time.Unix(0, 0)
	_ = session.BeforeFrame(now, func(Event) error { return nil })
	if !session.NeedsCheckpoint() {
		t.Fatal("checkpoint not requested")
	}
	if err := session.AfterFrame(now.Add(time.Millisecond), time.Millisecond, image.NewRGBA(image.Rect(0, 0, 1, 1))); err == nil || session.Result.Status != "failed" || !session.Done() {
		t.Fatal("checkpoint mismatch passed")
	}
}

func TestImageHashIgnoresStridePadding(t *testing.T) {
	image1 := image.NewRGBA(image.Rect(0, 0, 2, 2))
	image2 := image.NewRGBA(image.Rect(0, 0, 3, 2)).SubImage(image.Rect(0, 0, 2, 2))
	if ImageHash(image1) != ImageHash(image2) {
		t.Fatal("stride padding changed canonical hash")
	}
}

func TestQuantilesDoNotClampLongStalls(t *testing.T) {
	result := timing([]time.Duration{time.Millisecond, 2 * time.Millisecond, time.Second})
	if result.P95MS != 1000 || result.P99MS != 1000 || result.Over100MS != 1 {
		t.Fatalf("timing=%+v", result)
	}
}

func TestVariableQuantumUsesActualAdvances(t *testing.T) {
	session, err := New(testScenario(), "synthetic", "jit", 16*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(0, 0)
	for _, quantum := range []time.Duration{16, 20, 25, 30} {
		if err := session.BeforeFrame(now, func(Event) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if err := session.SetStepQuantum(quantum * time.Millisecond); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Millisecond)
		if err := session.AfterFrame(now, time.Millisecond, nil); err != nil {
			t.Fatal(err)
		}
	}
	if !session.Result.VariableFrameQuantum || session.Result.Measurement.GuestMS != 75 || session.Result.Segments[0].GuestMS != 36 {
		t.Fatalf("variable guest clock was multiplied by the initial quantum: %+v", session.Result)
	}
	if err := session.SetStepQuantum(0); err == nil {
		t.Fatal("zero quantum accepted")
	}
}
