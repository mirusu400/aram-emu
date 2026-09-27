package framebench

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"sort"
	"time"
)

type Timing struct {
	MeanMS    float64 `json:"mean_ms"`
	P50MS     float64 `json:"p50_ms"`
	P95MS     float64 `json:"p95_ms"`
	P99MS     float64 `json:"p99_ms"`
	MaxMS     float64 `json:"max_ms"`
	Over100MS uint64  `json:"over_100_ms"`
}

type Segment struct {
	FirstFrame     uint64  `json:"first_frame"`
	LastFrame      uint64  `json:"last_frame"`
	Frames         uint64  `json:"frames"`
	WallMS         float64 `json:"wall_ms"`
	GuestMS        float64 `json:"guest_ms"`
	GuestWallRatio float64 `json:"guest_wall_ratio"`
	StepTiming     Timing  `json:"step_timing"`
}

type CheckpointResult struct {
	Frame          uint64 `json:"frame"`
	SHA256         string `json:"sha256"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
	Matches        bool   `json:"matches"`
}

type Result struct {
	Runtime               string             `json:"runtime,omitempty"`
	MachineImplementation string             `json:"machine_implementation,omitempty"`
	ProfileID             string             `json:"profile_id,omitempty"`
	Schema                int                `json:"schema"`
	ScenarioID            string             `json:"scenario_id"`
	ScenarioSHA256        string             `json:"scenario_sha256"`
	InputSHA256           string             `json:"input_sha256"`
	Kind                  string             `json:"kind"`
	Status                string             `json:"status"`
	Error                 string             `json:"error,omitempty"`
	GOOS                  string             `json:"goos"`
	GOARCH                string             `json:"goarch"`
	GoVersion             string             `json:"go_version"`
	RequestedCPU          string             `json:"requested_cpu"`
	ResolvedCPU           string             `json:"resolved_cpu,omitempty"`
	FrameQuantumNS        int64              `json:"frame_quantum_ns"`
	QuantumSource         string             `json:"quantum_source"`
	VariableFrameQuantum  bool               `json:"variable_frame_quantum"`
	AudioDrain            string             `json:"audio_drain,omitempty"`
	CompletedFrames       uint64             `json:"completed_frames"`
	InputEvents           uint64             `json:"input_events"`
	WarmupFrames          uint64             `json:"warmup_frames"`
	Measurement           Segment            `json:"measurement"`
	Segments              []Segment          `json:"segments"`
	Checkpoints           []CheckpointResult `json:"checkpoints"`
	AudioSHA256           string             `json:"audio_sha256,omitempty"`
	AudioFrames           uint64             `json:"audio_frames,omitempty"`
}

// Session is owned by the frame worker, never the UI thread. Input frame 1
// means immediately before the first StepFrame/RunFrame after starting.
type Session struct {
	Scenario       Scenario
	Result         Result
	frame          uint64
	event          int
	checkpoint     int
	measuredAt     time.Time
	segmentAt      time.Time
	segmentFirst   uint64
	timings        []time.Duration
	segmentTimings []time.Duration
	quantum        time.Duration
	segmentGuest   time.Duration
	measuredGuest  time.Duration
}

func New(scenario Scenario, kind, cpu string, quantum time.Duration) (*Session, error) {
	if err := scenario.Validate(); err != nil {
		return nil, err
	}
	if quantum <= 0 || quantum > time.Minute {
		return nil, fmt.Errorf("invalid guest quantum")
	}
	encoded, err := canonicalScenario(scenario)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	return &Session{Scenario: scenario, quantum: quantum, Result: Result{
		Schema: 1, ScenarioID: scenario.ID, ScenarioSHA256: hex.EncodeToString(sum[:]),
		InputSHA256: scenario.SHA256, Kind: kind, Status: "running",
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GoVersion: runtime.Version(),
		RequestedCPU: cpu, FrameQuantumNS: int64(quantum), WarmupFrames: scenario.WarmupFrames,
	}}, nil
}

func (s *Session) Done() bool { return s.frame >= s.Scenario.Frames || s.Result.Status != "running" }

// SetStepQuantum accepts the runtime's actual advancement for this step.
// Variable Java advances must not be multiplied by the initial quantum.
func (s *Session) SetStepQuantum(quantum time.Duration) error {
	if quantum <= 0 || quantum > time.Minute {
		return s.Fail("invalid step quantum")
	}
	if quantum != s.quantum {
		s.Result.VariableFrameQuantum = true
	}
	s.quantum = quantum
	return nil
}

func (s *Session) BeforeFrame(now time.Time, queue func(Event) error) error {
	if s.Done() {
		return fmt.Errorf("benchmark already stopped")
	}
	if s.frame == 0 {
		s.segmentAt, s.segmentFirst = now, 1
	}
	if s.frame == s.Scenario.WarmupFrames {
		s.measuredAt = now
	}
	for s.event < len(s.Scenario.Events) && s.Scenario.Events[s.event].Frame == s.frame+1 {
		if err := queue(s.Scenario.Events[s.event]); err != nil {
			return s.Fail("input replay failed")
		}
		s.event++
		s.Result.InputEvents++
	}
	return nil
}

func (s *Session) NeedsCheckpoint() bool {
	return s.checkpoint < len(s.Scenario.Checkpoints) && s.Scenario.Checkpoints[s.checkpoint].Frame == s.frame+1
}

func (s *Session) AfterFrame(now time.Time, elapsed time.Duration, frame image.Image) error {
	if elapsed < 0 {
		return s.Fail("negative step duration")
	}
	s.frame++
	s.Result.CompletedFrames = s.frame
	s.segmentTimings = append(s.segmentTimings, elapsed)
	s.segmentGuest += s.quantum
	if s.frame > s.Scenario.WarmupFrames {
		s.timings = append(s.timings, elapsed)
		s.measuredGuest += s.quantum
	}
	if s.checkpoint < len(s.Scenario.Checkpoints) && s.Scenario.Checkpoints[s.checkpoint].Frame == s.frame {
		checkpoint := s.Scenario.Checkpoints[s.checkpoint]
		got := ImageHash(frame)
		matches := got != "" && (checkpoint.SHA256 == "" || checkpoint.SHA256 == got)
		s.Result.Checkpoints = append(s.Result.Checkpoints, CheckpointResult{s.frame, got, checkpoint.SHA256, matches})
		s.checkpoint++
		if !matches {
			return s.Fail(fmt.Sprintf("frame %d checkpoint mismatch", s.frame))
		}
	}
	if s.frame%s.Scenario.SegmentFrames == 0 || s.frame == s.Scenario.Frames {
		s.Result.Segments = append(s.Result.Segments, s.segment(s.segmentFirst, s.frame, s.segmentAt, now, s.segmentTimings, s.segmentGuest))
		s.segmentFirst, s.segmentAt, s.segmentTimings = s.frame+1, now, nil
		s.segmentGuest = 0
	}
	if s.frame == s.Scenario.Frames {
		s.Result.Measurement = s.segment(s.Scenario.WarmupFrames+1, s.frame, s.measuredAt, now, s.timings, s.measuredGuest)
		s.Result.Status = "complete"
	}
	return nil
}

func (s *Session) Fail(message string) error {
	s.Result.Status, s.Result.Error = "failed", message
	return fmt.Errorf("%s", message)
}

func (s *Session) segment(first, last uint64, start, end time.Time, samples []time.Duration, guest time.Duration) Segment {
	wall := end.Sub(start)
	frames := uint64(len(samples))
	result := Segment{FirstFrame: first, LastFrame: last, Frames: frames, WallMS: float64(wall) / float64(time.Millisecond), GuestMS: float64(guest) / float64(time.Millisecond), StepTiming: timing(samples)}
	if wall > 0 {
		result.GuestWallRatio = float64(guest) / float64(wall)
	}
	return result
}

func timing(samples []time.Duration) Timing {
	if len(samples) == 0 {
		return Timing{}
	}
	ordered := append([]time.Duration(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	var total time.Duration
	var stalls uint64
	for _, sample := range samples {
		total += sample
		if sample > 100*time.Millisecond {
			stalls++
		}
	}
	ms := func(sample time.Duration) float64 { return float64(sample) / float64(time.Millisecond) }
	quantile := func(percent int) float64 { return ms(ordered[(len(ordered)*percent+99)/100-1]) }
	return Timing{ms(total) / float64(len(samples)), quantile(50), quantile(95), quantile(99), ms(ordered[len(ordered)-1]), stalls}
}

func ImageHash(frame image.Image) string {
	if frame == nil || frame.Bounds().Empty() {
		return ""
	}
	bounds := frame.Bounds()
	hash := sha256.New()
	if rgba, ok := frame.(*image.RGBA); ok {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			offset := rgba.PixOffset(bounds.Min.X, y)
			_, _ = hash.Write(rgba.Pix[offset : offset+bounds.Dx()*4])
		}
	} else {
		row := make([]byte, bounds.Dx()*4)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				pixel := color.RGBAModel.Convert(frame.At(x, y)).(color.RGBA)
				i := (x - bounds.Min.X) * 4
				row[i], row[i+1], row[i+2], row[i+3] = pixel.R, pixel.G, pixel.B, pixel.A
			}
			_, _ = hash.Write(row)
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}
