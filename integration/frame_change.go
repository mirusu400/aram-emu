package integration

import (
	"bytes"
	"image"

	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-frontend/frontend"
)

// coreFramePresenter is the optional aram-core contract for reading the
// current frame together with a sequence that only changes when the pixels
// change. A core that implements it has already done the change tracking, so
// the adapter neither copies nor compares the framebuffer per host tick.
type coreFramePresenter interface {
	FramePresentation() (image.Image, uint64)
}

type coreVideoPresenter interface {
	VideoPresentation() aramcore.VideoPresentation
}

// presentedVideoFrame republishes the core's own presentation under the
// frontend's frame sequence.
func (backend *Backend) presentedVideoFrame(
	presenter coreFramePresenter,
) frontend.VideoFrame {
	frame, presentation := presenter.FramePresentation()
	return backend.publishVideoFrame(frame, presentation, 0, 0)
}

func (backend *Backend) presentedTimedVideoFrame(
	presenter coreVideoPresenter,
) frontend.VideoFrame {
	presentation := presenter.VideoPresentation()
	return backend.publishVideoFrame(
		presentation.Image,
		presentation.Sequence,
		presentation.GuestNS,
		presentation.Generation,
	)
}

func (backend *Backend) publishVideoFrame(
	frame image.Image,
	presentation uint64,
	guestNS int64,
	generation uint64,
) frontend.VideoFrame {
	if frame == nil || frame.Bounds().Dx() <= 0 || frame.Bounds().Dy() <= 0 {
		return frontend.VideoFrame{}
	}
	backend.mu.Lock()
	if backend.frameSequence == 0 || presentation != backend.lastPresentation {
		backend.lastPresentation = presentation
		backend.frameSequence++
	}
	sequence := backend.frameSequence
	backend.mu.Unlock()
	return frontend.VideoFrame{
		Image:      frame,
		Sequence:   sequence,
		GuestNS:    guestNS,
		Generation: generation,
	}
}

// frameChanged reports whether frame differs from the last one published to
// the frontend, and remembers the frame when it does. Callers hold backend.mu.
//
// This runs on the host UI thread on every tick, so its cost is charged
// directly to the host frame budget. Comparing the packed pixels is both exact
// and far cheaper than hashing them: hash/fnv mixes one byte at a time, which
// cost roughly half a millisecond per tick for a 240x320 frame - enough to
// matter on a handset-class CPU, and paid whether or not the guest drew
// anything. frameFingerprint remains the fallback for the rare image type that
// is not tightly packed RGBA.
func (backend *Backend) frameChanged(frame image.Image) bool {
	pixels, packed := packedFramePixels(frame)
	if !packed {
		backend.lastFramePixels = nil
		fingerprint := frameFingerprint(frame)
		if backend.lastFrameHash != 0 && fingerprint == backend.lastFrameHash {
			return false
		}
		backend.lastFrameHash = fingerprint
		return true
	}
	backend.lastFrameHash = 0
	if backend.lastFramePixels != nil &&
		bytes.Equal(backend.lastFramePixels, pixels) {
		return false
	}
	backend.lastFramePixels = append(backend.lastFramePixels[:0], pixels...)
	return true
}

// packedFramePixels exposes an RGBA frame's contiguous pixel bytes. A frame
// whose rows are padded, or that is not RGBA at all, reports false so the
// caller can fall back to a general comparison.
func packedFramePixels(frame image.Image) ([]byte, bool) {
	rgba, ok := frame.(*image.RGBA)
	if !ok {
		return nil, false
	}
	bounds := rgba.Bounds()
	stride := bounds.Dx() * 4
	if rgba.Stride != stride || len(rgba.Pix) != stride*bounds.Dy() {
		return nil, false
	}
	return rgba.Pix, true
}
