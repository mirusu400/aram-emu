# Cross-platform plan

The UI toolkit supports the shared render/input layer, but host packaging and
CPU backend availability are separate concerns.

| Target | Shared frontend | Host layer | Core baseline | Current gate |
|---|---|---|---|---|
| Windows amd64 | Ebitengine | Desktop executable and native picker | Pure Go; native backend optional | Local executable starts; CI required |
| Linux amd64 | Ebitengine | Desktop app, X11/Wayland packaging | Pure Go; native backend optional | Headless CI with Xvfb |
| macOS amd64/arm64 | Ebitengine | Signed/notarized app later | Pure Go; native backend optional | CI build/test before packaging |
| Android arm64 | Ebitengine mobile AAR | Native Activity, SAF, lifecycle, signing | Portable backend required | AAR builds; native host remains |
| iOS arm64 | Ebitengine XCFramework | UIKit host, document picker, signing | Portable backend required | Designed, not yet bound in CI |
| WebAssembly | Optional later | Browser host | Interpreter only | Exploratory, not a release target |

## Desktop

Windows, Linux, and macOS share screen composition, menus, settings, commands,
keyboard/gamepad mapping, and backend interaction. Build-tagged files provide
native dialogs and window behavior. Packaging remains platform-specific:

- Windows: executable, installer, associations, drag-and-drop;
- Linux: AppImage or Flatpak after dependency and sandbox testing;
- macOS: universal app, entitlements, signing, and notarization.

## Android

The frontend exports an Ebitengine mobile game into an AAR. A native Android
host owns:

- Activity and view lifecycle;
- Storage Access Framework document and directory selection;
- persisted URI permissions and cache copies where seekable access is needed;
- audio focus, controller/touch setup, intents, and background policy;
- APK/AAB packaging, signing, and store metadata.

The local foundation build has produced the AAR. That proves the shared Go UI
cross-compiles; it does not yet prove a complete installable Android app.

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
