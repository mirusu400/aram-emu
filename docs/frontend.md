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
  properties, compatibility reporting, and an attachable debug bundle.
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

## Memory Search

`Tools > Memory Search` searches the loaded game's guest regions through the
core cheat engine, independently of published cheat catalog availability.
Choose a region, numeric type (`u8/i8/u16/i16/u32/i32/u64/i64/f32/f64`), and
comparison. Integers accept decimal or explicitly prefixed `0x` hexadecimal,
including signed values; floats must be finite and fit their selected type.

First scan supports exact comparisons or an unknown initial value. Next scan
filters the existing candidates by value, increase, decrease, change, or no
change. The search type and region stay fixed until another first scan.
Results are read live in pages of 32 while comparison baselines stay unchanged
until the next successful scan. Close the panel to play, then reopen it to
continue the same search.

Select a result, review its current value, enter a new value, and apply it.
Writes validate the exact displayed bits immediately before applying. A changed
value is reported as a conflict and refreshed for review. Read-only regions and
unreadable addresses produce explicit errors. New search clears the baseline;
reset, state restoration, stopping, closing, or replacing the game invalidates
it and rejects requests from previous sessions.

The adapter serializes these operations with frame execution and lifecycle.
The core uses region snapshots plus candidate bitmaps; its paged APIs are
bounded by `MaxScanBytes` (128MiB by default), and each page is capped at 256.
Legacy materialized APIs retain `MaxResults` (2,000,000 by default) and their
previous-value semantics. A 32MiB u32 unknown scan retains 8,388,608 candidates
using a 32MiB snapshot plus a 1MiB bitmap; refinement temporarily needs both
the previous and new snapshots and bitmaps. A region left without candidates
releases its snapshot and is not read again. A result page that cannot be read
is reported with the search controls kept, so New search remains available.

## Debug export

`Tools > Export Debug Bundle...` (`Ctrl+Shift+D`) always exports the
frontend event log and manifest, even when no input is loaded or backend
diagnostic collection fails. The integrated adapter adds `core.json` and
`core.log` with input identity, CPU registers, the last execution result,
guest logs, KTF/WIPI trace tails, and runtime-specific fault context.

The ZIP does not include source bytes, host input paths, guest memory,
framebuffer pixels, save data, persistence, or proprietary media. Build and
host metadata contain no hostname. Files are size-bounded and checksummed in
the manifest before users attach the bundle to an issue.
