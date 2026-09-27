// Package framebench contains bounded, frame-indexed benchmark contracts.
// It has no windowing dependency and is shared by the headless core baseline
// and the opt-in full-product browser benchmark. It does not emulate a guest.
package framebench

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const MaxScenarioBytes = 1 << 20

type Event struct {
	Frame   uint64 `json:"frame"`
	Control string `json:"control"`
	Pressed bool   `json:"pressed"`
}

type Checkpoint struct {
	Frame  uint64 `json:"frame"`
	SHA256 string `json:"sha256,omitempty"`
}

type Scenario struct {
	Schema        int          `json:"schema"`
	ID            string       `json:"id"`
	SHA256        string       `json:"sha256"`
	Frames        uint64       `json:"frames"`
	WarmupFrames  uint64       `json:"warmup_frames"`
	SegmentFrames uint64       `json:"segment_frames"`
	Events        []Event      `json:"events,omitempty"`
	Checkpoints   []Checkpoint `json:"checkpoints,omitempty"`
}

func ReadScenario(reader io.Reader) (Scenario, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxScenarioBytes+1))
	if err != nil {
		return Scenario{}, err
	}
	if len(data) > MaxScenarioBytes {
		return Scenario{}, errors.New("scenario exceeds size limit")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var scenario Scenario
	if err := decoder.Decode(&scenario); err != nil {
		return Scenario{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Scenario{}, errors.New("scenario must contain one JSON object")
	}
	return scenario, scenario.Validate()
}

func validHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func (s Scenario) Validate() error {
	if s.Schema != 1 || s.ID == "" || len(s.ID) > 80 {
		return errors.New("invalid scenario schema or privacy-safe ID")
	}
	for _, char := range s.ID {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			return errors.New("scenario ID must be filename-safe ASCII")
		}
	}
	if !validHash(s.SHA256) {
		return errors.New("scenario requires an exact lowercase SHA-256")
	}
	if s.Frames == 0 || s.Frames > 1_000_000 || s.WarmupFrames >= s.Frames || s.SegmentFrames == 0 || s.SegmentFrames > s.Frames {
		return errors.New("invalid frame, warmup, or segment bounds")
	}
	if len(s.Events) > 100_000 || len(s.Checkpoints) > 10_000 {
		return errors.New("too many scenario entries")
	}
	pressed := make(map[string]bool)
	var previous uint64
	for _, event := range s.Events {
		if event.Frame == 0 || event.Frame > s.Frames || event.Frame < previous || event.Control == "" || len(event.Control) > 255 || strings.ContainsAny(event.Control, "\x00\r\n") {
			return errors.New("invalid or unordered input event")
		}
		if pressed[event.Control] == event.Pressed {
			return fmt.Errorf("unbalanced input transition for %q", event.Control)
		}
		pressed[event.Control] = event.Pressed
		previous = event.Frame
	}
	for _, held := range pressed {
		if held {
			return errors.New("scenario leaves an input pressed")
		}
	}
	previous = 0
	for _, checkpoint := range s.Checkpoints {
		if checkpoint.Frame == 0 || checkpoint.Frame > s.Frames || checkpoint.Frame <= previous || checkpoint.SHA256 != "" && !validHash(checkpoint.SHA256) {
			return errors.New("invalid or unordered checkpoint")
		}
		previous = checkpoint.Frame
	}
	return nil
}
