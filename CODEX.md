# Codex project guide

## Repository role

`aram-emu` is the ARAM ecosystem, integration, packaging, and release
repository. It must not grow duplicate copies of core or frontend code.

Sibling ownership:

- `aram-core`: headless emulation, loaders, profiles, state, debugger backend;
- `aram-frontend`: cross-platform presentation, host input, settings, and
  emulator workflows;
- `anycall_magichole`: reverse-engineering evidence and executable reference;
- `aram-emu`: adapters between the public contracts, product configuration,
  platform hosts, packaging, release manifests, and project-wide plans.

Every repository has its own `CODEX.md`. Read that file before changing a
sibling repository.

## Mission

ARAM is a general-purpose Korean feature-phone emulator. It is not a
`MinigameQVGAOEM` launcher and not a KTF-only compatibility shim. Compatibility
must grow through explicit WIPI version, carrier, manufacturer, device, and
title profiles.

The two product modes are:

- application mode: ARM/Thumb WIPI applications with WIPI/OEM HLE;
- system mode: user-supplied firmware with CPU, memory, interrupt, timer,
  storage, display, keypad, and device models.

Share stable infrastructure between the modes, but do not force both through
one machine implementation.

## Dependency direction

The final application or native host may import `aram-frontend` and
`aram-core`. The frontend must not import concrete core internals. An adapter
owned by integration translates `frontend.Backend` operations to an exported
core machine contract.

```text
desktop/mobile host -> aram-frontend <- integration adapter -> aram-core
                                                        |
                                             app or system backend
```

Avoid circular dependencies. Cross-repository contracts are versioned. A
breaking contract change requires coordinated commits, migration notes, and a
pinned integration revision.

## Cross-platform rules

- Windows, Linux, macOS, and Android are required product targets.
- iOS stays inside the design boundary and becomes required after an iOS host
  is added.
- Default core packages remain pure Go and headless.
- Native/JIT CPU implementations are optional backends behind build tags.
- A portable interpreter is the compatibility fallback on mobile and hosts
  where executable memory or native libraries are unavailable.
- Desktop-specific dialogs never enter mobile builds.
- Android/iOS hosts own document pickers, permissions, lifecycle, packaging,
  and signing; the shared frontend accepts document handles or cache paths.

## Product requirements

Do not remove a generic emulator command to simplify a single-title demo.
File/open, recent files, firmware selection, pause/reset/stop, states, rewind,
speed controls, scaling, screenshots, controller mapping, audio, cheats,
patches, debugger, logs, and compatibility reporting remain part of the
product contract. Unsupported operations remain visible and disabled.

## Evidence and data policy

- Never commit firmware, ROMs, games, keys, dumps, IDA databases, extracted
  fonts, or proprietary media.
- Use synthetic unit fixtures.
- Private integration tests may locate the evidence checkout through
  `ARAM_REFERENCE_REPO` or user-owned data through `ARAM_TEST_DATA`.
- Never modify a user-supplied image in place.
- Do not silently download firmware, games, or keys.
- A title-specific result never proves carrier-wide or WIPI-wide support.

## Release discipline

A release candidate requires:

1. pinned core and frontend revisions;
2. clean CI on required desktop targets and Android;
3. third-party dependency and license review;
4. reproducible compatibility records with hashes and profiles;
5. save-state schema and backend identifiers when states are enabled;
6. crash-safe handling of malformed and unsupported input;
7. no proprietary material in source, artifacts, or test fixtures.

Do not add a repository license or ship a linked emulator release until the
combined dependency and distribution licensing review is recorded.
