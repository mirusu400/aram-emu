# Cross-platform plan

The UI toolkit supports the shared render/input layer, but host packaging and
CPU backend availability are separate concerns.

| Target | Shared frontend | Host layer | Core baseline | Current gate |
|---|---|---|---|---|
| Windows amd64 | Ebitengine | Icon-bearing executable and native picker | Pure Go; native backend optional | Local executable starts; CI required |
| Linux amd64 | Ebitengine | Desktop app, X11/Wayland packaging | Pure Go; native backend optional | Headless CI with Xvfb |
| macOS amd64/arm64 | Ebitengine | Unsigned app bundle; signing/notarization later | Pure Go; native backend optional | Icon-bearing arm64 app bundles in CI |
| Android arm64 | Ebitengine mobile AAR | Native Activity, SAF, lifecycle, signing | Portable backend required | Branded Nightly (debug key) and Stable (release key) APKs in CI |
| iOS arm64 | Ebitengine XCFramework | UIKit host, document picker, signing | Portable backend required | Designed, not yet bound in CI |
| WebAssembly | Optional later | Browser host | Interpreter only | Exploratory, not a release target |

## Desktop

Windows, Linux, and macOS share screen composition, menus, settings, commands,
keyboard/gamepad mapping, and backend interaction. Build-tagged files provide
native dialogs and window behavior. Packaging remains platform-specific:

- Windows: icon-bearing executable, installer, associations, drag-and-drop;
- Linux: AppImage or Flatpak after dependency and sandbox testing;
- macOS: icon-bearing arm64 app bundle now; universal binaries, entitlements,
  signing, and notarization later.

## Android

The integrated product exports its Ebitengine mobile game into an AAR. The
native Android host in `android/` owns:

- Activity and view lifecycle;
- Storage Access Framework document and directory selection;
- persisted URI permissions and cache copies where seekable access is needed;
- audio focus, controller/touch setup, intents, and background policy;
- APK/AAB packaging, signing, and store metadata.

CI builds and lints arm64/x86_64 APKs on two channels. Every push to `main`
produces the rolling Nightly APK (`ARAM Nightly`, application ID
`io.github.mirusu400.aram.nightly`, `nightly.keystore` signature); a published
GitHub release produces the Stable APK (`ARAM`, application ID
`io.github.mirusu400.aram`, `stable.keystore` signature). Each build verifies its
launchable Activity, JNI ABI, embedded component revisions, launcher label,
package name, and signing certificate before publishing. The Activity uses SAF
to copy a selected provider
document into private storage before passing a seekable path to the ordinary
integration backend. It also forwards View/Send intents, lifecycle state,
audio focus, touch, keyboard, and gamepad events.

Both channels are repo-signed with intentionally public keystores, so they
provide install and update continuity, not release authenticity. Store-grade
production signing (a privately held Stable key), device instrumentation,
accessibility, and broader Android-device validation remain release gates.

## iOS

The same shared game can become an XCFramework. A native UIKit host owns
document access, security-scoped resources, lifecycle, signing, and store
packaging. iOS cannot assume JIT or executable-memory privileges, so a portable
CPU interpreter is a prerequisite for a credible release.

## CPU portability

Unicorn or another native CPU engine may accelerate supported desktop and
mobile hosts, but it cannot be the only implementation. The planned order is:

1. deterministic portable ARM/Thumb interpreter;
2. optional Unicorn/native backend with matching architectural behavior;
3. differential tests that run the same instruction traces on both;
4. backend-tagged save-state context and compatibility checks.

Product support is the intersection of frontend, host, core, CPU backend, and
packaging. A green shared-UI build alone is not a complete platform release.
