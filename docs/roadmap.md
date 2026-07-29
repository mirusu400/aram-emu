# Implementation roadmap

Statuses use `done`, `active`, `planned`, and `research`. A phase is complete
only when its exit gate is reproducible; a visual demo is not sufficient.

| Phase | Status | Exit gate |
|---|---|---|
| 0. Repository foundation | done | Core/frontend/integration/evidence ownership split; private repos, project guides, CI |
| 1. Application-mode execution | done | One authorized native WIPI title reaches its first frame through ordinary File/Open |
| 2. WIPI runtime breadth | planned | Versioned API matrix, graphics/input/audio/storage/timer coverage, conformance fixtures |
| 3. Carrier and device profiles | planned | KTF, SKT, and LGT evidence separated; Samsung and other OEM quirks profile-driven |
| 4. Emulator product features | planned | States, replay, speed controls, controllers, screenshots, cheats, patches, debugger |
| 5. Android product | planned | Native host installs, SAF open works, lifecycle/audio/input tests pass on arm64 device |
| 6. Full-system firmware | research | Reproducible user-supplied Magic Hole snapshot/AMSS boot milestone with documented devices |
| 7. Compatibility and releases | planned | Hash-keyed automated reports and reproducible signed desktop/mobile release candidates |

## Phase 1 - executable WIPI application mode

- Move the known ABHS/EADS loaders and profile contracts into the clean core.
- Define guest address space, imports, relocations, entry point, stacks, and
  fault reporting.
- Implement a deterministic ARM/Thumb interpreter baseline.
- Add an optional Unicorn backend only behind an interchangeable CPU contract.
- Define WIPI/OEM call dispatch and the first graphics, input, timer, storage,
  and audio services.
- Connect the integration adapter to the generic frontend open flow.
- Compare milestones with the `anycall_magichole` reference without copying
  proprietary input into product repositories.

Exit: an authorized reference title reaches a rendered first frame from
File/Open and the same boot trace is repeatable in a headless test.

## Phase 2 - WIPI SDK and runtime coverage

- Inventory API surface by WIPI revision and distinguish standard, carrier,
  OEM, device, and title-specific behavior.
- Turn each understood API into typed contracts, synthetic tests, error cases,
  and deterministic state rules.
- Cover graphics, fonts, events, keypad/touch, timers, sound, storage, resource
  loading, threads, synchronization, networking stubs, and lifecycle.
- Record unsupported calls explicitly instead of returning fake success.
- Extract `aram-wipi-sdk` only when clean headers/stubs and example apps are
  independently useful and legally reviewed.

Exit: a published internal coverage matrix and a multi-title synthetic
conformance suite pass without title-specific code in standard services.

## Phase 3 - compatibility profiles

- Model WIPI version, carrier, manufacturer, device, and title as independent
  layered profiles.
- Add SKT and LGT from direct evidence rather than extrapolating KTF behavior.
- Key title overrides and patches by strong source hash.
- Require expected-original bytes for binary patches.

Exit: at least one authorized input per claimed carrier follows the same loader
and machine path, with differences expressed only through profiles.

## Phase 4 - complete emulator workflow

- Save/load slots, named states, rewind, deterministic input replay.
- Fast-forward, frame advance, speed control, pause/reset/stop.
- Keyboard, gamepad, controller hotplug, touch overlays, and per-title mapping.
- Native-resolution screenshots, filters, layouts, rotation, and aspect rules.
- Audio device, mute, latency, resampling, and synchronization.
- Cheat manager, memory search, patch manager, debugger, trace, logs, title
  properties, and compatibility report.

Exit: feature behavior is backend-capability driven, state schemas are
versioned, and corrupted inputs/states fail without damaging user data.

## Phase 5 - Android and later iOS

- Create a native Android host around the verified AAR.
- Implement SAF documents/directories, URI persistence, intents, lifecycle,
  audio focus, controller input, touch overlay, settings, and crash reporting.
- Add instrumented device tests and reproducible APK/AAB packaging.
- Repeat the native-host pattern for an iOS XCFramework after the portable CPU
  path and Android lifecycle are stable.

Exit: a user-selected authorized package launches through the normal mobile
document flow, survives pause/resume, and produces no desktop-only dependency.

## Phase 6 - original firmware boot

This work runs beside application mode; it does not block early WIPI releases.

1. Catalog Magic Hole firmware partitions, boot stages, SoC, memory map, and
   peripherals from reproducible evidence.
2. Start from controlled memory snapshots to validate CPU, memory, interrupts,
   display, keypad, timers, and storage.
3. Reach a documented AMSS entry milestone.
4. Add earlier OEMSBL/QCSBL boot stages as secure-boot, flash, and device
   behavior become understood.
5. Model telephony safely with no real-network bridge by default.

Exit is milestone-based. "Firmware boots" requires a documented input hash,
device profile, start state, trace, visible result, and known missing hardware.

## Immediate next queue

Completed foundation milestones:

1. Portable ARM/Thumb interpreter skeleton and instruction tests in
   `aram-core`.
2. Bounded source/container inspection, layered profile data, and structured
   frontend failures.
3. Integration adapter, local workspace, and desktop product entry point in
   `aram-emu`.
4. Synthetic and authorized Magic Hole inputs mapped through the ordinary open
   path; the entry dispatcher executes and returns reproducibly.
5. Independent `aram-test` compatibility laboratory with synthetic gates,
   authorized commercial-corpus loops, privacy-safe reports, deltas, triage,
   and scheduled CI.
6. Hash-keyed reference profile dispatch through bootstrap, setup, start,
   preload, and a deterministic first visible frame.

Next:

1. Pin and continuously test coordinated core, frontend, and integration
   revisions in private CI.
2. Attach an authorized multi-title corpus to `aram-test` and define
   hash-keyed minimum expectations without committing any input.
3. Generalize the hash-keyed reference runtime into typed WIPI, carrier, OEM,
   device, and title services; replace opaque success fallbacks with evidenced
   behavior or explicit unsupported results.
4. Add framebuffer evidence and deterministic input replay beyond the first
   visible frame.
5. Expand the portable interpreter from the observed subset according to
   corpus faults, with an instruction regression test for every added encoding.
6. Create the Android native-host repository only when the adapter can launch
   multiple profiled titles, avoiding a disconnected empty shell.
