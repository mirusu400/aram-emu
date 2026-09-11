package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-frontend/frontend"
)

func TestJavaFramePublicationDoesNotClaimBootOrInputResponse(t *testing.T) {
	result := probeResult{Status: "ok_alive", Level: "loads"}
	copyDiagnostics(&result, integration.Diagnostics{Java: &integration.JavaDiagnostics{
		Runtime: "j2me", MainClass: "Synthetic", Started: true, HasDisplay: true, Instructions: 14,
		PresentCount: 2, FramebufferSHA256: strings.Repeat("a", 64), FrameValid: true,
	}})
	if !observeJavaFrame(&result, 3) || result.Status != "ok_frame" || result.Level != "loads" ||
		result.FirstFrameSlice != 3 || result.Java.Runtime != "j2me" || result.Java.Instructions != 14 ||
		result.Image != nil || result.LastExecution != nil || result.WIPI != nil {
		t.Fatalf("Java publication result = %+v", result)
	}
	result.InputEvents = 2
	result.PostFrameSlices = 10
	updatePostInteractionMilestone(&result)
	if result.Level != "loads" {
		t.Fatalf("unverified Java input falsely promoted milestone to %s", result.Level)
	}
}

func TestJavaFrameRequiresExecutionAndValidatedPublication(t *testing.T) {
	for _, java := range []*javaResult{
		nil, {},
		{Started: true, Instructions: 2},
		{Started: true, Instructions: 2, PresentCount: 1},
		// Shared host initialization can publish a blank framebuffer even when
		// the return-only MIDlet never sets a Displayable.
		{Started: true, Instructions: 2, PresentCount: 1, FrameValid: true},
		{HasDisplay: true, Instructions: 2, PresentCount: 1, FrameValid: true},
		{Started: true, HasDisplay: true, PresentCount: 1, FrameValid: true},
	} {
		result := probeResult{Java: java, Status: "ok_alive", Level: "loads"}
		if observeJavaFrame(&result, 1) || result.Status != "ok_alive" || result.FirstFrameSlice != 0 {
			t.Fatalf("incomplete Java frame evidence accepted: %+v", java)
		}
	}
}

func TestRecognizedLegacyPlatformsRemainUnsupportedNotSuccessful(t *testing.T) {
	err := &frontend.BackendError{Kind: frontend.FailureUnsupportedProfile, Err: errors.New("execution not implemented")}
	for _, format := range []string{"brew-package", "gnex-sgs", "j2me", "unknown", ""} {
		status, level, kind := classifyError(err, format)
		wantLevel := "recognized"
		if format == "unknown" || format == "" {
			wantLevel = ""
		}
		if status != "unsupported_format" || level != wantLevel || kind != frontend.FailureUnsupportedProfile {
			t.Fatalf("format %q: status=%s level=%s kind=%s", format, status, level, kind)
		}
	}
}
