# Codex project guide

## Mission

ARAM is a general-purpose Korean feature-phone emulator, not a
`MinigameQVGAOEM` launcher and not a KTF-only compatibility shim. It must grow
through explicit WIPI, carrier, manufacturer, device, and title profiles while
keeping the core reusable.

The sibling `anycall_magichole` repository is the evidence and reference-oracle
project. ARAM is the clean Go product implementation.

## Product modes

- **Application mode:** load WIPI packages and run native applications quickly
  with ARM/Thumb execution plus WIPI/OEM HLE.
- **System mode:** boot user-supplied original firmware with low-level CPU,
  memory, interrupt, timer, storage, display, keypad, and device models.

Do not force both modes through one implementation. They should share frontend,
input, audio, state, debugger, patch, and profile infrastructure.

## Frontend requirements

The desktop frontend is a product surface, not a temporary test harness. Keep a
conventional emulator workflow comparable to mature console emulators:

- persistent top menus for File, Emulation, View, Tools, and Help;
- Open File and Open Firmware Directory directly from the File menu;
- recent files, drag-and-drop, command-line file association, and reopen;
- start, pause, resume, stop, reset, frame advance, fast-forward, and speed
  control;
- save/load state slots, named states, screenshots, recording, rewind, and
  deterministic input replay;
- integer scaling, aspect-ratio control, fullscreen, rotation, filters, and
  screen-layout presets;
- keyboard and gamepad mapping, controller hotplug, vibration, and per-game
  input profiles;
- volume, mute, audio latency, and device selection;
- cheat manager, memory search, patch manager, debugger, logs, compatibility
  report, and title properties;
- clear empty, loading, running, paused, stopped, and fault states;
- no title should bypass the ordinary File/Open workflow.

Features may initially be disabled while the backend is absent, but their
commands and architecture must not be removed to simplify a single-game demo.
See `docs/frontend.md` for the menu contract.

## Architecture rules

- `internal/core` owns the backend-neutral machine contract.
- `internal/cpu` owns replaceable CPU backends. Unicorn is an initial backend,
  not a permanent dependency of every package.
- `internal/loader` parses untrusted inputs with strict bounds checks.
- WIPI-standard behavior, OEM behavior, carrier behavior, device quirks, and
  title patches remain separate.
- Frontend packages issue commands; they do not emulate APIs or mutate guest
  memory directly.
- Time, randomness, storage, and input must be virtualizable for deterministic
  replay and save states.
- Avoid per-instruction host callbacks outside debugger/trace modes.
- Game-specific patches are hash-keyed data with expected-original-byte checks,
  not anonymous hard-coded addresses in the core.

## Source and data policy

- Do not commit firmware, ROMs, memory dumps, games, fonts extracted from
  devices, IDA databases, or proprietary audio/image assets.
- Test with synthetic fixtures in the public tree.
- Private integration tests may read user-owned data through
  `ARAM_TEST_DATA` or locate the evidence repository through
  `ARAM_REFERENCE_REPO`.
- Do not silently download firmware or keys.
- Never modify a user-supplied source image in place.

## Compatibility policy

Every compatibility claim must name:

1. input hash;
2. loader/container type;
3. carrier/device profile;
4. backend and ARAM commit;
5. reproducible input sequence;
6. observed result or failure.

A title-specific success is not platform-wide support. KTF evidence does not
automatically prove SKT or LGT behavior.

## Go and test conventions

- Keep packages small and dependency direction explicit.
- Return typed errors with offsets for malformed binary data.
- Use table-driven tests and fuzz parsers that accept external bytes.
- `go test ./...` and `go vet ./...` must pass before commit.
- Format Go code with `gofmt`.
- Add regression coverage before fixing a compatibility crash.
- Keep GUI-free packages testable on headless CI.

## Licensing checkpoint

Do not add a repository license or ship linked emulator binaries until the
Unicorn, frontend, audio, and optional full-system backend licenses have been
reviewed as a combined distribution. Record third-party versions and licenses
in a checked-in dependency manifest before the first release.
