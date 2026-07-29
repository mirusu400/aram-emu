# Frontend product contract

The implementation lives in
[`aram-frontend`](https://github.com/mirusu400/aram-frontend). This document is
the product-level acceptance contract used by integration and releases.

## Persistent workflows

- File: open package, open firmware, recent entries, close, associations,
  drag-and-drop, command-line input, and mobile document intents.
- Emulation: start, pause/resume, stop, reset, frame advance, fast-forward,
  speed control, states, state slots, and rewind.
- View: fullscreen, integer scaling, aspect, fit, rotation, layouts, filters,
  and screenshots.
- Input/audio: keyboard, gamepad, touch, per-title profiles, hotplug, volume,
  mute, latency, and output-device selection.
- Tools: cheats, memory search, patches, debugger, trace, logs, title
  properties, and compatibility reporting.
- Help: documentation, issue reporting, build information, and about.

Unavailable operations stay visible and disabled with an explanation. No title
may bypass the ordinary open pipeline through a hidden hard-coded launcher.

## Observable states

The product distinguishes empty, selecting, inspecting, loading, ready,
running, paused, stopped, backend-unavailable, guest-faulted, malformed-input,
and unsupported-profile states.

An actionable error includes the selected input identity, detected format,
profile decision, backend, and reason. Errors do not collapse into a blank
screen.

## Presentation invariants

- Guest framebuffer pixels are distinct from window chrome.
- Scaling and filters do not mutate guest-native screenshots.
- Frontend code does not read or write guest memory directly.
- Desktop and mobile layouts may differ while command IDs and capabilities
  remain consistent.
- Settings and recent items fail safely when paths or permissions expire.
- Mobile UI never assumes an Android content URI is a filesystem path.
