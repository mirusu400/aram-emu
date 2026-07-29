# Architecture

ARAM separates product presentation, integration, and emulation:

```text
 Windows / Linux / macOS app       Android / iOS native host
                \                         /
                 +----- aram-frontend ---+
                         Backend contract
                                |
                    aram-emu integration adapter
                                |
                           aram-core
                       /               \
             WIPI application       firmware system
              ARM + WIPI HLE        ARM + device models
                       \               /
                      versioned profiles

 authorized private corpus -> aram-test -> aram-probe -> integration adapter
                                  |
                         report / delta / triage
```

## Component boundaries

### Frontend

The frontend owns presentation state, emulator commands, framebuffer layout,
audio delivery, host input, settings, file-selection entry points, overlays,
and tool windows. It sees an exported backend contract rather than guest
memory or a concrete CPU.

### Integration

The integration layer selects an application or system backend, translates
frontend commands to machine operations, pins compatible component versions,
and owns desktop/mobile product entry points. This is the only layer expected
to import both sibling code modules.

### Core

The core owns loaders, memory maps, machine lifecycle, CPU execution, WIPI/OEM
services, device models, deterministic state, debugging, patches, and profiles.
It has no window, display-server, Android Activity, or UIKit dependency.

### Compatibility laboratory

`aram-test` treats `aram-probe` as a black-box product boundary. It owns
synthetic gates, user-authorized commercial corpus discovery, per-input
caches, cross-revision deltas, and failure clustering. It does not import or
duplicate core, frontend, or integration implementation.

## Application mode

1. Inspect an untrusted package through bounds-checked loaders.
2. Resolve WIPI, carrier, manufacturer, device, and title profiles.
3. Map ARM/Thumb images and runtime objects into a 32-bit guest address space.
4. Execute through a selected CPU backend.
5. Dispatch WIPI-standard and profile-selected OEM calls.
6. Expose framebuffer, audio, input, storage, timers, and machine state through
   backend-neutral contracts.

An initial success on one KTF title is evidence for that input and profile
only. SKT, LGT, Java, other WIPI revisions, and other manufacturers require
their own profiles and tests.

## System mode

1. Identify and validate a user-supplied flash, firmware, or memory snapshot.
2. Instantiate the matching SoC and device profile.
3. Map ROM, RAM, interrupts, timers, storage, display, keypad, and required
   peripherals.
4. Progress from controlled memory-snapshot execution to AMSS entry, then
   earlier boot stages as hardware knowledge improves.
5. Keep telephony virtual and disconnected from real mobile networks by
   default.

Full firmware boot is not architecturally impossible, but it is a separate,
long-running hardware-emulation effort and is not implied by application-mode
WIPI compatibility.

## Determinism and tooling

Virtual time, seeded randomness, storage, input queues, and synthetic network
responses are serializable machine state. Save states record:

- core and backend identity/version;
- profile and source hashes;
- CPU registers and backend context;
- RAM and mapped-object state;
- WIPI/HLE handles and services;
- device, interrupt, timer, audio, and storage state.

Cheats and patches are hash-keyed data with expected-original checks. Debugger
and memory tools use explicit core services rather than unchecked access to a
backend implementation.
