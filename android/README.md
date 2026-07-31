# Android product host

The Android app wraps the integrated `aram-emu/mobile` Ebitengine binding. It
is not the standalone frontend preview: the AAR statically includes the pinned
`aram-core`, `aram-frontend`, and integration adapter.

`MainActivity` owns the Android-specific boundary:

- Ebitengine view and Activity lifecycle;
- Go mobile context and app-private settings initialization;
- Storage Access Framework document selection;
- private, bounded copies of provider documents for the Go backend;
- incoming View/Send document intents;
- audio focus and gamepad/touch delivery through Ebitengine.

## Local Nightly build

Prerequisites are Go, `ebitenmobile`, JDK 17, Android SDK 36, Android NDK
28.2.13676358 or newer, and Gradle 8.14.1. NDK r28+ is required so the Go JNI
library is compatible with Android devices using 16 KB memory pages.

```powershell
go install github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile@v2.9.9
New-Item -ItemType Directory -Force android/app/libs | Out-Null
ebitenmobile bind -target android/arm64 -androidapi 23 -trimpath `
  -ldflags="-s -w" -javapkg io.github.mirusu400.aram `
  -o android/app/libs/aram.aar ./mobile
gradle --no-daemon -p android :app:lintDebug :app:assembleDebug
```

The output is
`android/app/build/outputs/apk/debug/app-debug.apk`. It is debug-signed and
uses application ID `io.github.mirusu400.aram.nightly`, so it can be installed
beside a future store-signed Stable app. CI publishes the same arm64 APK as
`aram-android-arm64.apk`.

Nightlies use the repository-owned `nightly.keystore` so a newer CI build can
upgrade an installed Nightly. Its credentials are intentionally public and
provide continuity, not release authenticity; this key must never sign a
Stable or store build.

For a local x86_64 emulator smoke test, bind with
`-target android/amd64` and set `ARAM_ANDROID_ABIS=x86_64` for the Gradle
invocation. Published Nightlies deliberately contain only arm64-v8a.

The selected document is copied into the app-private `files/imports`
directory. ARAM never edits the provider-owned source. Android may grant a
persistable URI permission, but emulation uses only the private copy so the
backend always receives a seekable filesystem path.

Frontend settings and debug exports use the app-private `files/config`
directory. The Activity initializes the Go runtime context and this storage
root before Ebitengine creates the shared frontend shell.
