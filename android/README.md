# Android product host

The Android app wraps the integrated `aram-emu/mobile` Ebitengine binding. It
is not the standalone frontend preview: the AAR statically includes the pinned
`aram-core`, `aram-frontend`, and integration adapter.

`MainActivity` owns the Android-specific boundary:

- Ebitengine view and Activity lifecycle;
- Go mobile context and app-private settings initialization;
- Storage Access Framework document selection;
- Storage Access Framework create-document export for backups and debug ZIPs;
- private, bounded copies of provider documents for the Go backend;
- per-game pinned Home screen shortcuts for imported titles;
- incoming View/Send document intents;
- verified `aram.mir.sh/player/` App Links and `aram://open` deep links;
- audio focus and gamepad/touch delivery through Ebitengine;
- handing downloaded product updates to the system package installer.

Android also supplies audio output properties, worker performance hints, and
PCM pump thread priority. See [Android runtime performance](../docs/android-runtime-performance.md)
for buffering, input scheduling, diagnostics, and device verification.

The `Mobile.pause()` / `Mobile.resume()` bridge also ends and restarts foreground
PostHog analytics sessions. Backgrounding sends the final performance window and
session duration without waiting for network I/O on the Activity thread. Usage
analytics includes input file names (without folder paths) and performance
summaries while the existing setting is enabled. See the frontend
[analytics schema](https://github.com/mirusu400/aram-frontend/blob/main/docs/analytics.md)
for event properties.

## Local build (Nightly and Stable)

Prerequisites are Go, `ebitenmobile`, JDK 17, Android SDK 36, Android NDK
28.2.13676358 or newer, and Gradle 9.1.0. NDK r28+ is required so the Go JNI
library is compatible with Android devices using 16 KB memory pages.

```powershell
$aramEbitenVersion = (go list -m -f '{{.Version}}' github.com/hajimehoshi/ebiten/v2).Trim()
go install "github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile@${aramEbitenVersion}"
New-Item -ItemType Directory -Force android/app/libs | Out-Null
ebitenmobile bind -target android/arm,android/arm64,android/amd64 -androidapi 23 -trimpath `
  -ldflags="-s -w" -javapkg io.github.mirusu400.aram `
  -o android/app/libs/aram.aar ./mobile
gradle --no-daemon -p android :app:testGithubDebugUnitTest :app:lintGithubDebug :app:assembleGithubDebug
```

Run these commands from the `aram-emu` repository root with the required sibling
checkouts from `go.work` present. The binder version is read from the product's
Go module, as in CI, because its Java glue must match the linked Ebitengine API.

The Gradle build derives its density-specific and adaptive launcher icons from
`../aram-frontend/frontend/assets/icon.png`, the same pinned artwork used by
the desktop window. Set `ARAM_ICON_SOURCE` to an absolute PNG path only when
building from a workspace with a different sibling layout.

`:app:assembleGithubDebug` produces the Nightly channel at
`android/app/build/outputs/apk/github/debug/app-github-debug.apk`: launcher label `ARAM
Nightly`, application ID `io.github.mirusu400.aram.nightly`, signed with
`nightly.keystore`. `:app:assembleGithubRelease` produces the Stable channel at
`android/app/build/outputs/apk/github/release/app-github-release.apk`: launcher label `ARAM`,
base application ID `io.github.mirusu400.aram`, signed with `stable.keystore`.
The differing application IDs let the two channels install side by side; the
per-channel launcher label is injected by `resValue` in `app/build.gradle`, not
`strings.xml`. CI publishes the ARMv7+arm64+x86_64 APK for whichever channel the
event selects as `aram-android-universal.apk`. ARMv7 requires NEON, as required
by the current NDK. GitHub builds require Android 6.0 (API 23) or newer; Play
builds require Android 7.0 (API 24) or newer.

The standalone app also requires OpenGL ES 3.0, as required by the existing
Ebitengine renderer. ARMv7, NEON, and the Android version alone do not establish
GPU compatibility. ES 2.0-only retro handhelds can use the
[ARMv7 libretro core](../docs/libretro.md) with a compatible RetroArch renderer;
that headless core does not depend on Ebitengine or ES 3.0.

Each channel keeps a fixed repository-owned signer so a newer CI build can
upgrade an installed app in place: Nightly uses `nightly.keystore`, Stable uses
`stable.keystore` (`CN=ARAM`). Both keystores' credentials are intentionally
public and provide update continuity, not release authenticity; the Nightly key
must never sign the Stable channel or vice versa. A future store-grade Stable
release would swap `stable.keystore` for a privately held key, a one-time re-key
that existing Stable installs cannot update across.

For a smaller local arm64-only build, bind with `-target android/arm64` and set
`ARAM_ANDROID_ABIS=arm64-v8a` for the Gradle invocation.

For an ARMv7-only device build, use `-target android/arm` and set
`ARAM_ANDROID_ABIS=armeabi-v7a`. Rebuild the AAR whenever its target set changes:
Gradle's ABI filter cannot create a missing Go library. The Play AAR must use
the same targets and retain its `SelfUpdateDisabled` linker setting.

## ABI verification

Verify the complete JNI set and ELF machine types before distributing an archive:

```powershell
python -m unittest discover -s scripts -p test_verify_android_native.py
python scripts/verify_android_native.py android/app/libs/aram.aar
python scripts/verify_android_native.py android/app/build/outputs/apk/github/debug/app-github-debug.apk
python scripts/verify_android_native.py android/app/build/outputs/bundle/playRelease/app-play-release.aab
```

The verifier requires all three ABIs by default. Pass `--abis armeabi-v7a`
when checking an intentionally single-ABI local build. It checks every packaged
native library, requires 16 KB LOAD alignment for arm64/x86_64, and accepts
4 KB LOAD alignment for ARMv7.

CI runs the portable core tests as ARMv7 machine code through QEMU with a
NEON-capable CPU, in addition to building the actual Android JNI/audio bindings.
ARMv7 uses the existing portable Go JIT for the default `fastest` CPU setting;
the AArch64 native JIT remains available on arm64. Saved guest state uses the
guest CPU architecture rather than the host ABI.

Before shipping, exercise the APK on ARMv7 Android 6.0 and the optimized Play
APK or device APKs generated from its AAB on ARMv7 Android 7.0. Check normal
document opening, rendering, input, audio, pause/resume, save/load, and an update
over an existing install. Retest arm64 and x86_64, including a 64-bit device with
16 KB pages. A desktop x86 emulator does not prove ARMv7 execution. Modern
Android Emulator releases do not support the old ARM32 system images; use a
compatible emulator or an ARMv7 device. Old ARM32 emulator images may expose
only ES 2.0, so they can test the libretro core but cannot validate the standalone
app's renderer. An ARM translation test on a newer x86 emulator is additional
coverage, not a replacement for an ARMv7 device with ES 3.0.

The universal sideload APK includes one additional native library, increasing
download size. Play delivers ABI-specific APKs from the AAB. Large packages and
firmware can still exhaust memory on low-memory 32-bit devices; runtime support
does not imply the performance or memory capacity of a recent arm64 handset.

## Document handling

The selected document is copied into the app-private `files/imports`
directory. ARAM never edits the provider-owned source. Android may grant a
persistable URI permission, but emulation uses only the private copy so the
backend always receives a seekable filesystem path.

To pin a game, open it once, return to ARAM Home, select its Recent row, and
tap **Create shortcut** (홈 화면 바로가기 만들기). Android 8.0+ asks the launcher to
add an icon bearing the game's name and its extracted game icon. A game with
no icon cannot be pinned. Tapping the icon opens and starts the private copy
directly. The launcher intent contains only a random ID; ARAM keeps the path
privately. Clearing app data or deleting that copy invalidates the shortcut.

An App Link carries the web player's `app` HTTPS URL and mandatory `sha256`
digest. ARAM downloads at most 32 MiB without credentials, rejects non-HTTPS
redirects and digest mismatches, then stores the verified package below
`files/config/ARAM/downloads`. Reopening the same link reuses the verified local
copy, and the integration backend's SHA-256-keyed save data is restored in the
same way as for a manually selected file. `aram.mir.sh/.well-known/assetlinks.json`
must contain every release signing certificate, including a future Google Play
App Signing certificate, before that signer can receive verified App Links.

## Play Store advertising

The public source tree and GitHub/sideload APKs contain no AdMob SDK, no AdMob
app ID, and no production ad unit ID. The Play flavor is the only flavor that
contains banner advertising:

- `playDebug` always uses Google's public demo app and banner IDs, so it is
  safe for development and policy-safe device testing.
- `playRelease` reads the actual values from CI only. Add these GitHub Actions
  secrets before publishing a release: `ARAM_ADMOB_APP_ID` and
  `ARAM_ADMOB_BANNER_AD_UNIT_ID`. The release workflow fails before building
  the AAB if either is absent.

For a published `vX.Y.Z` GitHub release, CI sets the Android APK and Play AAB
`versionName` to `X.Y.Z`. The `versionCode` is the build workflow's increasing
run number; verify it exceeds the highest code already uploaded in Play Console
before publishing a new release. The Play AAB is attached to the GitHub release
after the release workflow succeeds, for manual upload to Play Console.

The Play flavor requires Android 7.0 (API 24) or newer for Google Play automatic
protection. GitHub Stable and Nightly builds still support Android 6.0 (API 23).
The shared AAR's `ebitenmobile -androidapi 23` setting does not set the Play
app's minimum SDK; the Play flavor overrides it in `app/build.gradle`.

The Play host refreshes consent using Google's User Messaging Platform (UMP)
on every launch, displays a configured consent form when required, and shows a
Privacy options button whenever UMP requires a persistent entry point. In the
AdMob console, create and publish the required Privacy & messaging messages
for the Play app before release.

Release variants use AGP 9 and R8 full-mode optimization with resource
shrinking. The ebitenmobile AAR supplies consumer rules for its JNI bindings;
the app does not add broad keep rules that would defeat DEX shrinking or
obfuscation. CI requires non-empty R8 mapping and removed-code reports and at
least one renamed class before accepting a Play AAB. A manual `build` workflow
run with `play_release_candidate` enabled exercises the signed Play release
path without creating a public GitHub release.

Frontend settings and working artifact files use the app-private
`files/config` directory. The Activity initializes the Go runtime context and
this storage root before Ebitengine creates the shared frontend shell. Save
backups and debug ZIPs are then copied through the system create-document UI to
a Documents, Downloads, or cloud-provider location selected by the user.

When the Activity pauses, the Go bridge flushes the running title's writable
storage into its SHA-256-keyed private save file. This covers Android process
death after backgrounding, where the desktop-style backend close path may
never run.

## In-app product updates

The frontend's Updates settings download the published
`aram-android-universal.apk` for the selected channel into the app-private
`files/updates` directory. The Go layer verifies size and SHA-256 digest, then
the Activity hands the package to the system package installer through the
non-exported `UpdateProvider` with a per-intent read grant; the user confirms
the update there while ARAM keeps running. Android asks once per source for
the `REQUEST_INSTALL_PACKAGES` "install unknown apps" approval. Because the
installer reads the package after the hand-off, `files/updates` is cleared on
the next launch rather than immediately. In-app updates work within a channel
because CI signs every Nightly with `nightly.keystore` and every Stable with
`stable.keystore`; the two channels are separate apps (different application IDs
and signers), so moving between them is a manual install, not an in-app update.
