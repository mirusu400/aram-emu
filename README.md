# ARAM

**ARAM - Archived Runtime for ARM Mobiles**

[![build](https://github.com/mirusu400/aram-emu/actions/workflows/build.yml/badge.svg)](https://github.com/mirusu400/aram-emu/actions/workflows/build.yml)

ARAM is a cross-platform emulator project for Korean feature-phone software.
It targets both fast application-level WIPI emulation and, as a longer-term
research track, booting user-supplied original device firmware.

This repository is the product integration and release repository.
The core and frontend are intentionally separate:

| Repository | Responsibility | Primary output |
|---|---|---|
| [`aram-core`](https://github.com/mirusu400/aram-core) | Headless loaders, profiles, machine contracts, CPU and WIPI runtime | Go library |
| [`aram-frontend`](https://github.com/mirusu400/aram-frontend) | Shared emulator UI plus desktop/mobile host boundaries | Desktop app and mobile library |
| [`aram-emu`](https://github.com/mirusu400/aram-emu) | Architecture, roadmap, integration adapter, packaging, releases | ARAM product |
| `aram-test` | Synthetic and authorized commercial-corpus orchestration, reports, deltas, and triage | Compatibility laboratory |

Product integration imports the two sibling modules instead of copying their
implementation. A local Go workspace connects sibling checkouts while their
contracts are still changing.

## Product modes

- **Application mode:** load a WIPI package, execute its ARM/Thumb guest code,
  and provide WIPI, carrier, OEM, display, audio, input, storage, and timing
  services through high-level emulation.
- **System mode:** load a user-supplied firmware image, instantiate a concrete
  device/SoC profile, and progressively boot snapshots, AMSS, and eventually
  the original boot chain as hardware coverage permits.

The modes share the frontend, input mapping, audio, profiles, save-state,
debugger, patch, and compatibility infrastructure. They do not pretend to be
the same backend.

## Platform target

Windows, Linux, and macOS are first-class desktop targets. Android is a
first-class mobile target through an Ebitengine AAR and native Android host.
iOS is designed into the boundary and follows after Android. The core keeps a
portable interpreter path so that an optional native/JIT backend does not
decide which hosts ARAM can support.

Current foundation status:

- `aram-core` tests natively and cross-compiles for Android/arm64;
- `aram-frontend` tests on desktop, builds a Windows executable, and binds to
  an Android AAR;
- `aram-emu` now owns the adapter and desktop product entry point that connect
  the frontend's ordinary open request to the core application factory;
- synthetic EADS and the authorized Magic Hole reference both reach and
  execute their mapped native entry points; the profiled reference completes
  bootstrap, setup, start, preload, and its first visible frame deterministically;
- the independent `aram-test` repository consumes the headless product probe
  for synthetic gates and authorized commercial-corpus loop engineering;
- conventional File, Emulation, View, Tools, and Help commands remain in the
  frontend even when the backend does not implement them yet;
- users can export one checksummed debug ZIP with redacted frontend logs and
  bounded core CPU/runtime diagnostics for issue triage;
- broad native instruction and WIPI/OEM service coverage, multi-title
  first-frame compatibility, production-grade state workflows, cheats, and
  firmware boot remain roadmap work, not completed compatibility claims.

Run the integrated desktop product from this checkout:

```powershell
go run ./cmd/aram
go run ./cmd/aram path\to\authorized-input.dat
```

Packaged desktop builds act as their own bootstrap. On the first launch,
choosing Nightly downloads the rolling integrated `aram-emu` Release, verifies
its GitHub digest, installs it below the platform ARAM configuration directory,
and restarts into that runtime. The installed archive contains the compatible
core and frontend revisions recorded in `BUILD-INFO.txt`; they are not dynamic
plugins. Choosing Stable installs the latest non-prerelease when one exists and
otherwise continues with the bundled build. After an update-driven restart,
ARAM opens the game picker automatically.

The bootstrap never overwrites its running executable. It keeps versioned,
content-addressed runtimes under `ARAM/runtime/versions` and records the active
one in `ARAM/runtime/current.json`; launching the original downloaded binary
delegates to that active runtime.

Build or invoke the black-box compatibility probe:

```powershell
go run ./cmd/aram-probe -input path\to\authorized-input.dat
```

Corpus discovery, caching, comparison, and reports belong to the sibling
`aram-test` repository rather than the product tree.

## Continuous builds

Every push and pull request runs the desktop tests and builds ARAM on Windows
x64, Linux x64, and macOS arm64. Download the compressed `aram-*` build from
the workflow run's **Artifacts** section. CI artifacts are retained for 14
days. A successful push to `main` also updates the rolling `nightly`
prerelease with the exact same three archives and `SHA256SUMS.txt`. The release
notes link back to the source commit and workflow run. Publishing a
non-nightly GitHub release builds the tagged product and attaches its archives
and checksums to that release.

The product workflow checks out pinned, known-compatible revisions of the
public `aram-core` and `aram-frontend` repositories next to this repository,
so the local `go.work` and `replace` directives resolve reproducibly. A manual
workflow run may override either dependency ref for integration testing. Each
archive contains `BUILD-INFO.txt` with the exact revisions used.

## Project documents

- [Repository ownership](docs/repositories.md)
- [Architecture and dependency direction](docs/architecture.md)
- [Core/frontend integration contract](docs/integration.md)
- [Cross-platform strategy](docs/platforms.md)
- [Implementation roadmap and release gates](docs/roadmap.md)
- [Frontend product requirements](docs/frontend.md)
- [Compatibility evidence policy](docs/compatibility.md)
- [Codex project guide](CODEX.md)

ARAM never distributes firmware, commercial games, keys, memory dumps, device
fonts, or extracted proprietary assets. Users provide material they are
authorized to use.
