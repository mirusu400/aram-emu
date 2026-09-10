# Android libretro core

ARAM's libretro host is a headless product adapter over `aram-core`. It does not
import the Ebitengine frontend. The Android artifact uses the portable CPU
backend and is built as a standard libretro shared object.

## Build

Requirements:

- Go 1.25 or newer;
- Android SDK with NDK 27 or newer;
- `ANDROID_HOME`, `ANDROID_SDK_ROOT`, or `ANDROID_NDK_HOME` configured. The
  script also detects the newest NDK below `%LOCALAPPDATA%\Android\Sdk\ndk`.

From `aram-emu`:

```powershell
.\scripts\build-libretro-android.ps1 -Abi arm64-v8a
```

The core is written to:

```text
build/libretro/android/arm64-v8a/aram_libretro_android.so
```

An x86_64 build is available for Android emulators:

```powershell
.\scripts\build-libretro-android.ps1 -Abi x86_64
```

## Device smoke test

The smoke test builds the x86_64 core and a small Android libretro host, pushes
both through ADB, and exercises the public ABI with a synthetic WIPI image:

```powershell
.\scripts\test-libretro-android.ps1
```

Pass `-Serial emulator-5554` when more than one device is connected.

## Controls

| RetroPad | Handset control |
|---|---|
| D-pad | Direction keys |
| A / B | OK / Clear |
| X / Y | Left / right soft key |
| Start | Menu |
| L / R | Star / Hash |
| L2 / R2 | Send / End |

Hold Select for the numeric layer: X=1, Up=2, Y=3, Left=4, A=5, Right=6,
L=7, Down=8, R=9, B=0, L2=Star, and R2=Hash.

## Runtime behavior

- Content is supplied in memory (`need_fullpath=false`) and inspected by the
  same `aram-core/loader` used by other product hosts.
- Video is XRGB8888 at the guest-native resolution. Geometry changes are
  reported through `RETRO_ENVIRONMENT_SET_GEOMETRY`.
- Audio is signed PCM16 stereo at 44.1 kHz. Mono guest output is duplicated to
  stereo.
- Persistent WIPI storage is written below the frontend's save directory as
  `aram/<content-sha256>.aramsave`.
- Save states use a fixed 64 MiB libretro buffer with an ARAM envelope that
  binds the state to the loaded content hash. A state larger than the buffer is
  rejected rather than truncated.

Whole-phone firmware mode is not exposed by this application-mode core.
Raptor Java save states remain unavailable because `aram-core` does not yet
serialize that adapter; gameplay and persistent title storage are unaffected.
