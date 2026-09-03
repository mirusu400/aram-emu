<p align="center">
  <img src="https://aram.mir.sh/assets/icon.png" width="120" alt="ARAM">
</p>

<h1 align="center">ARAM</h1>

<p align="center">
  <b>Archived Runtime for ARM Mobiles</b><br>
  Bring Korean feature-phone (WIPI) software back to life, on your computer, phone, or straight in the browser.
</p>

<p align="center">
  <a href="https://aram.mir.sh/en/"><b>🌐 Website</b></a> ·
  <a href="https://aram.mir.sh/en/#play"><b>▶ Play in your browser</b></a> ·
  <a href="https://github.com/mirusu400/aram-emu/releases"><b>⬇ Download</b></a>
</p>

<p align="center">
  <a href="https://github.com/mirusu400/aram-emu/actions/workflows/build.yml"><img src="https://github.com/mirusu400/aram-emu/actions/workflows/build.yml/badge.svg" alt="build"></a>
</p>

---

## What is ARAM?

ARAM is an emulator for **Korean feature-phone software**, the small games and
apps that ran on 2000s WIPI handsets. Point it at a program you're allowed to
use, and ARAM runs it again, on modern hardware.

> 한국 피처폰(WIPI) 시절의 게임과 앱을 지금의 PC·폰·브라우저에서 다시 실행하는 에뮬레이터입니다.

It runs on **Windows, Linux, macOS, and Android**, and there's a full
**in-browser version**, no install required.

## Try it now

- **▶ Play in your browser:** open **[aram.mir.sh](https://aram.mir.sh/en/#play)**,
  press *Launch ARAM*, then `File ▸ Open` a file you own. Everything runs locally
  in your browser; nothing is uploaded.
- **⬇ Download the app:** grab the latest build for your system.

| | Download (latest stable) |
|---|---|
| **Windows** | [aram-windows-amd64.zip](https://github.com/mirusu400/aram-emu/releases/latest/download/aram-windows-amd64.zip) |
| **macOS** (Apple silicon) | [aram-macos-arm64.tar.gz](https://github.com/mirusu400/aram-emu/releases/latest/download/aram-macos-arm64.tar.gz) |
| **Linux** | [aram-linux-amd64.tar.gz](https://github.com/mirusu400/aram-emu/releases/latest/download/aram-linux-amd64.tar.gz) |
| **Android** | [aram-android-universal.apk](https://github.com/mirusu400/aram-emu/releases/latest/download/aram-android-universal.apk) |

Prefer the bleeding edge? Every change to the project is also published as a
[**Nightly**](https://github.com/mirusu400/aram-emu/releases/tag/nightly) build.
You can pick **Stable** or **Nightly** right on the [download page](https://aram.mir.sh/en/#download).

## What can it run?

ARAM has two ways to bring old software back:

### 📱 Run apps and games
Load a WIPI app or game and ARAM runs it directly, providing the phone services
it expects, display, sound, input, storage, and timing. **This works today.**

### 🔌 Boot a whole phone *(experimental)*
Point ARAM at a real phone's firmware and it boots the entire device, one
supported model at a time (currently the Samsung SCH-W830 and SCH-W860). This is
an ongoing research track.

## Features

- **Runs everywhere**, Windows, Linux, macOS, Android, and the browser.
- **No installer needed to try it**, the web version runs the real emulator in a tab.
- **Save states & rewind**, snapshot a game and jump back anytime.
- **Cheats, debugger, and patching**, the full toolbox for tinkering.
- **Custom controls**, remap your keyboard or gamepad, or use the on-screen keypad.
- **One-click bug reports**, export a redacted debug bundle and file an issue from inside the app.

## How honest is it?

Very. ARAM is under active development, and it tracks its progress in clear
steps: *recognized → loads → executes → first frame → playable → complete*. A few
reference titles already reach their first frame; broad game support and full
phone boot are still on the way. Every result is reported honestly, so you
always know exactly how far a title gets.

## Bring your own files

**You supply the software you want to run.** ARAM works with firmware, games,
and other material that you already own or are authorized to use, and it stays
an independent, community project.

---

## For developers

ARAM is split into focused, independent repositories that this one integrates
into the shipping product:

| Repository | Responsibility |
|---|---|
| [`aram-emu`](https://github.com/mirusu400/aram-emu) | Product integration, packaging, releases, roadmap *(this repo)* |
| [`aram-core`](https://github.com/mirusu400/aram-core) | Headless Go core, loaders, ARM/Thumb CPU, profiles, WIPI runtime, save states |
| [`aram-frontend`](https://github.com/mirusu400/aram-frontend) | Shared UI, desktop & mobile hosts, menus, input, overlays |
| [`aram-cheat`](https://github.com/mirusu400/aram-cheat) | Cheat database, keyed by the loaded-image hash |

Run the integrated desktop app from a checkout:

```powershell
go run ./cmd/aram
go run ./cmd/aram path\to\your-input.dat
```

Build the browser version:

```powershell
$env:GOOS = "js"; $env:GOARCH = "wasm"
go build -o web/aram.wasm ./cmd/aram-web
# then serve web/ (index.html + wasm_exec.js + aram.wasm)
```

Every push builds and tests ARAM on Windows, Linux, and macOS, binds the Android
product, and refreshes the rolling `nightly` release. Deeper documentation lives
in [`docs/`](docs/):

- [Architecture & dependency direction](docs/architecture.md)
- [Core / frontend integration contract](docs/integration.md)
- [Cross-platform strategy](docs/platforms.md)
- [Implementation roadmap & release gates](docs/roadmap.md)
- [Frontend requirements](docs/frontend.md)
- [Compatibility evidence policy](docs/compatibility.md)
- [Repository ownership](docs/repositories.md)
- [Android host & local APK build](android/README.md)
