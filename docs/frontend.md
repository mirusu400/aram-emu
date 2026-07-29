# Desktop frontend contract

ARAM keeps familiar desktop-emulator controls even while individual backends
are incomplete.

## File

- Open File... (`Ctrl+O`)
- Open Firmware Directory...
- Recent Files
- Close Title
- Exit

Opening a file performs type detection first, selects or requests a compatible
profile, and then creates a machine. Drag-and-drop and command-line paths use
the same code path. Recent entries that no longer exist remain removable but
must not crash the UI.

## Emulation

- Start (`F5`)
- Pause/Resume (`F6`)
- Stop (`F8`)
- Reset (`Ctrl+R`)
- Frame Advance
- Fast Forward
- Load State / Save State
- State Slot
- Rewind

Unavailable operations remain visible and disabled rather than disappearing.

## View

- Fullscreen (`F11`)
- Integer Scaling
- Preserve Aspect Ratio
- Fit Window
- Rotation
- Screen Layout
- Filter
- Screenshot

The emulator framebuffer is logically separate from window chrome. Scaling and
filters never alter guest pixels or screenshot-at-native-resolution output.

## Tools

- Cheat Manager
- Memory Search
- Patch Manager
- Debugger
- Controller Settings
- Audio Settings
- Title Properties
- Compatibility Report
- Logs

Tool windows communicate through debugger/state interfaces. They do not reach
into a specific CPU backend with unchecked casts.

## Help

- Documentation
- Report Issue
- About ARAM

## States

The frontend visibly distinguishes:

- no input loaded;
- inspecting;
- ready;
- running;
- paused;
- stopped;
- backend unavailable;
- guest fault;
- malformed or unsupported input.

Errors include the selected path, detected format, profile, and an actionable
reason. They must not be reduced to a blank screen.
