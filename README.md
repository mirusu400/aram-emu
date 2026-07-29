# ARAM

**ARAM - Archived Runtime for ARM Mobiles**

ARAM is a cross-platform emulator project for Korean feature-phone software.
It targets both fast application-level WIPI emulation and, as a longer-term
research track, booting user-supplied original device firmware.

This repository is the private ecosystem, integration, and release repository.
The core and frontend are intentionally separate:

| Repository | Responsibility | Primary output |
|---|---|---|
| [`aram-core`](https://github.com/mirusu400/aram-core) | Headless loaders, profiles, machine contracts, CPU and WIPI runtime | Go library |
| [`aram-frontend`](https://github.com/mirusu400/aram-frontend) | Shared emulator UI plus desktop/mobile host boundaries | Desktop app and mobile library |
| [`aram-emu`](https://github.com/mirusu400/aram-emu) | Architecture, roadmap, integration adapter, packaging, releases | ARAM product |
| [`anycall_magichole`](https://github.com/mirusu400/anycall_magichole) | Magic Hole reverse-engineering evidence and reference implementation | Research oracle |

Duplicated bootstrap code was removed from this repository after the split.
Product integration will import the two sibling modules instead of copying
their implementation.

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
- conventional File, Emulation, View, Tools, and Help commands remain in the
  frontend even when the backend does not implement them yet;
- native guest execution, broad WIPI API coverage, save states, cheats, and
  firmware boot remain roadmap work, not completed compatibility claims.

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
