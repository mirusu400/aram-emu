# ARAM

**ARAM — Archived Runtime for ARM Mobiles**

ARAM is a general-purpose emulator for Korean feature-phone software. The
project is being designed for both fast application-level WIPI emulation and a
future full-system firmware mode.

> Status: private bootstrap repository. The desktop shell, container
> inspection, architecture boundaries, and compatibility policy exist; ARM
> guest execution has not yet been ported from the reference implementation.

## Goals

- Load files directly from a conventional desktop emulator frontend.
- Run native WIPI-C ARM/Thumb applications through a fast HLE path.
- Support carrier and device profiles instead of one hard-coded KTF layout.
- Add original-firmware boot as a separate full-system path.
- Provide save states, input replay, cheats, patches, debugging, screenshots,
  gamepad mapping, and a compatibility database.
- Never require proprietary firmware or game files to be distributed with
  ARAM.

## Commands

```powershell
go run ./cmd/aram
go run ./cmd/aram gui
go run ./cmd/aram inspect path\to\file.dat
go test ./...
```

The default command opens the desktop shell. Its File menu can select a WIPI
package or firmware directory; the selected input is inspected and recorded in
the recent-file list. Emulator execution is introduced behind the `core.Machine`
interface rather than being embedded in the UI.

## Repository relationship

The sibling `anycall_magichole` repository is the reverse-engineering evidence
source and executable Python reference. ARAM is the clean product
implementation. Raw firmware, memory dumps, IDA databases, commercial games,
and extracted assets must not be committed here.

See [CODEX.md](CODEX.md), [architecture](docs/architecture.md), and the
[frontend contract](docs/frontend.md) before making broad changes.
