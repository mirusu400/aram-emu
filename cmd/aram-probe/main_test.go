package main

import (
	"errors"
	"slices"
	"testing"

	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-frontend/frontend"
)

func TestSplitControlsTrimsAndDropsEmptyItems(t *testing.T) {
	if got := splitControls(" ok, , left ,,soft-right "); !slices.Equal(
		got,
		[]string{"ok", "left", "soft-right"},
	) {
		t.Fatalf("splitControls() = %q", got)
	}
}

func TestClassifyErrorPreservesActionableCompatibilityStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		format     string
		wantStatus string
		wantLevel  string
	}{
		{
			name: "unsupported format",
			err: &frontend.BackendError{
				Kind: frontend.FailureUnsupportedProfile,
				Err:  errors.New("unsupported"),
			},
			format:     "java-archive",
			wantStatus: "unsupported_format",
			wantLevel:  "recognized",
		},
		{
			name: "malformed recognized input",
			err: &frontend.BackendError{
				Kind: frontend.FailureMalformedInput,
				Err:  errors.New("bad input"),
			},
			format:     "wipi-dat",
			wantStatus: "malformed_input",
			wantLevel:  "recognized",
		},
		{
			name: "unsupported instruction",
			err: &frontend.BackendError{
				Kind: frontend.FailureGuestFaulted,
				Err:  cpu.ErrUnsupportedInstruction,
			},
			format:     "eads",
			wantStatus: "unimplemented_instruction",
			wantLevel:  "loads",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, level, _ := classifyError(test.err, test.format)
			if status != test.wantStatus || level != test.wantLevel {
				t.Fatalf("classifyError() = %q, %q; want %q, %q",
					status, level, test.wantStatus, test.wantLevel)
			}
		})
	}
}
