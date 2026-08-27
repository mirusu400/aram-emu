package integration

import (
	"image"
	"image/color"
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
)

// The published sequence must only move when the guest actually redrew, and
// the check must not depend on hashing every pixel each tick.
func TestPackedFrameChangeDetectionSkipsIdenticalFrames(t *testing.T) {
	backend := &Backend{}
	frame := image.NewRGBA(image.Rect(0, 0, 6, 4))

	if !backend.frameChanged(frame) {
		t.Fatal("the first frame was not reported as changed")
	}
	if backend.frameChanged(frame) {
		t.Fatal("an unchanged frame was reported as changed")
	}
	if backend.lastFrameHash != 0 {
		t.Fatal("a packed frame fell back to the per-pixel fingerprint")
	}

	// The guest redraws into the same buffer, so the retained comparison copy
	// must not alias it.
	frame.SetRGBA(3, 2, color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff})
	if !backend.frameChanged(frame) {
		t.Fatal("an in-place guest redraw was missed")
	}
	if backend.frameChanged(frame) {
		t.Fatal("the redrawn frame was not adopted as the new baseline")
	}
}

func TestFrameChangeDetectionNoticesResolutionChanges(t *testing.T) {
	backend := &Backend{}
	if !backend.frameChanged(image.NewRGBA(image.Rect(0, 0, 6, 4))) {
		t.Fatal("the first frame was not reported as changed")
	}
	if !backend.frameChanged(image.NewRGBA(image.Rect(0, 0, 8, 4))) {
		t.Fatal("a resized frame was not reported as changed")
	}
}

func TestUnpackedFrameChangeDetectionFallsBackToTheFingerprint(t *testing.T) {
	backend := &Backend{}
	frame := image.NewNRGBA(image.Rect(0, 0, 5, 3))

	if !backend.frameChanged(frame) {
		t.Fatal("the first unpacked frame was not reported as changed")
	}
	if backend.lastFrameHash == 0 {
		t.Fatal("an unpacked frame did not record a fingerprint")
	}
	if backend.frameChanged(frame) {
		t.Fatal("an unchanged unpacked frame was reported as changed")
	}
	frame.SetNRGBA(2, 1, color.NRGBA{G: 0xff, A: 0xff})
	if !backend.frameChanged(frame) {
		t.Fatal("a redrawn unpacked frame was missed")
	}
}

func TestPackedFramePixelsRejectsPaddedRows(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 6, 4))
	sub, ok := frame.SubImage(image.Rect(0, 0, 3, 4)).(*image.RGBA)
	if !ok {
		t.Fatal("sub-image is not RGBA")
	}
	if _, packed := packedFramePixels(sub); packed {
		t.Fatal("a padded sub-image was treated as tightly packed")
	}
	if _, packed := packedFramePixels(frame); !packed {
		t.Fatal("a tightly packed frame was rejected")
	}
}

type presentingMachine struct {
	frame        *image.RGBA
	presentation uint64
}

func (m *presentingMachine) FramePresentation() (image.Image, uint64) {
	return m.frame, m.presentation
}

// A core that tracks its own presentations must drive the published sequence
// directly: the adapter neither copies nor compares the framebuffer.
func TestPresentedVideoFrameFollowsTheCorePresentationSequence(t *testing.T) {
	backend := &Backend{}
	machine := &presentingMachine{
		frame:        image.NewRGBA(image.Rect(0, 0, 4, 3)),
		presentation: 7,
	}

	first := backend.presentedVideoFrame(machine)
	if first.Sequence == 0 || first.Image != machine.frame {
		t.Fatalf("first published frame = %+v", first)
	}
	if repeat := backend.presentedVideoFrame(machine); repeat.Sequence != first.Sequence {
		t.Fatalf(
			"unchanged presentation published sequence %d, want %d",
			repeat.Sequence,
			first.Sequence,
		)
	}
	if backend.lastFramePixels != nil || backend.lastFrameHash != 0 {
		t.Fatal("a presenting core still paid for adapter-side change detection")
	}

	machine.presentation = 8
	if changed := backend.presentedVideoFrame(machine); changed.Sequence == first.Sequence {
		t.Fatal("a new core presentation kept the published sequence")
	}
}

func TestPresentedVideoFrameIgnoresAnEmptyFrame(t *testing.T) {
	backend := &Backend{}
	empty := &presentingMachine{frame: image.NewRGBA(image.Rect(0, 0, 0, 0)), presentation: 1}
	if got := backend.presentedVideoFrame(empty); got.Image != nil || got.Sequence != 0 {
		t.Fatalf("empty frame published %+v", got)
	}
}

type presentingCoreMachine struct {
	aramcore.Machine
	frame            *image.RGBA
	presentation     uint64
	guestNS          int64
	generation       uint64
	framebufferCalls int
}

func (m *presentingCoreMachine) FramePresentation() (image.Image, uint64) {
	return m.frame, m.presentation
}

func (m *presentingCoreMachine) VideoPresentation() aramcore.VideoPresentation {
	return aramcore.VideoPresentation{
		Image:      m.frame,
		Sequence:   m.presentation,
		GuestNS:    m.guestNS,
		Generation: m.generation,
	}
}

func (m *presentingCoreMachine) Framebuffer() image.Image {
	m.framebufferCalls++
	return m.frame
}

type unwrappingFrameMachine struct {
	aramcore.Machine
	frame            image.Image
	framebufferCalls int
}

func (m *unwrappingFrameMachine) Unwrap() aramcore.Machine {
	return m.Machine
}

func (m *unwrappingFrameMachine) Framebuffer() image.Image {
	m.framebufferCalls++
	return m.frame
}

// The product publishes a cheat wrapper rather than the application machine.
// Read-only presentation discovery must reach the inner machine without using
// the wrapper's copying Framebuffer fallback.
func TestVideoFrameFindsPresentationThroughWrapper(t *testing.T) {
	inner := &presentingCoreMachine{
		frame:        image.NewRGBA(image.Rect(0, 0, 4, 3)),
		presentation: 7,
		guestNS:      25_000_000,
		generation:   3,
	}
	wrapper := &unwrappingFrameMachine{
		Machine: inner,
		frame:   image.NewRGBA(image.Rect(0, 0, 9, 8)),
	}
	backend := &Backend{machine: wrapper}

	first := backend.VideoFrame()
	if first.Image != inner.frame || first.Sequence == 0 {
		t.Fatalf("first wrapped presentation = %+v", first)
	}
	if first.GuestNS != inner.guestNS || first.Generation != inner.generation {
		t.Fatalf("wrapped presentation timeline = %+v", first)
	}
	if wrapper.framebufferCalls != 0 || inner.framebufferCalls != 0 {
		t.Fatalf(
			"Framebuffer calls wrapper=%d inner=%d, want 0",
			wrapper.framebufferCalls,
			inner.framebufferCalls,
		)
	}
	if repeat := backend.VideoFrame(); repeat.Sequence != first.Sequence || repeat.Image != inner.frame {
		t.Fatalf("unchanged wrapped presentation = %+v, want sequence %d", repeat, first.Sequence)
	}

	inner.presentation++
	inner.guestNS += 16_000_000
	if changed := backend.VideoFrame(); changed.Sequence == first.Sequence || changed.Image != inner.frame {
		t.Fatalf("changed wrapped presentation = %+v", changed)
	}
}

// Unwrapping is only for optional read-only capability discovery. If the
// inner machine has no presenter, the ordinary wrapper Framebuffer contract
// remains authoritative.
func TestVideoFrameFallbackStaysOnWrapper(t *testing.T) {
	inner := &unwrappingFrameMachine{
		frame: image.NewRGBA(image.Rect(0, 0, 4, 3)),
	}
	wrapperFrame := image.NewRGBA(image.Rect(0, 0, 6, 5))
	wrapper := &unwrappingFrameMachine{
		Machine: inner,
		frame:   wrapperFrame,
	}
	backend := &Backend{machine: wrapper}

	got := backend.VideoFrame()
	if got.Image != wrapperFrame {
		t.Fatalf("fallback image = %v, want wrapper image", got.Image)
	}
	if wrapper.framebufferCalls != 1 || inner.framebufferCalls != 0 {
		t.Fatalf(
			"Framebuffer calls wrapper=%d inner=%d, want 1/0",
			wrapper.framebufferCalls,
			inner.framebufferCalls,
		)
	}
}

var (
	benchmarkVideoImage    image.Image
	benchmarkVideoSequence uint64
)

func benchmarkVideoFramePresentation(b *testing.B, wrapped bool) {
	inner := &presentingCoreMachine{
		frame:        image.NewRGBA(image.Rect(0, 0, 240, 320)),
		presentation: 1,
	}
	var machine aramcore.Machine = inner
	if wrapped {
		machine = &unwrappingFrameMachine{Machine: inner, frame: inner.frame}
	}
	backend := &Backend{machine: machine}
	_ = backend.VideoFrame()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		frame := backend.VideoFrame()
		benchmarkVideoImage = frame.Image
		benchmarkVideoSequence = frame.Sequence
	}
}

func BenchmarkVideoFramePresentationUnwrapped(b *testing.B) {
	benchmarkVideoFramePresentation(b, false)
}

func BenchmarkVideoFramePresentationWrapped(b *testing.B) {
	benchmarkVideoFramePresentation(b, true)
}
