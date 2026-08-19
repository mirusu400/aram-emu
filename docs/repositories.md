# Repository ownership

ARAM uses separate repositories where components have different portability,
release, or evidence requirements. The split is not a microservice exercise;
it prevents UI dependencies and reverse-engineering data from leaking into the
headless core.

## Active repositories

| Repository | Owns | Must not own |
|---|---|---|
| `aram-core` | Machine and CPU contracts, safe loaders, profiles, WIPI/OEM runtime, state and debugger services | Windows, Android, or Ebitengine UI code |
| `aram-frontend` | Shared Ebitengine screens, commands, settings, keyboard/gamepad/touch UI, platform host bridges | WIPI parsing, guest execution, device internals |
| `aram-emu` | Integration adapter, release manifests, platform packaging, project roadmap and acceptance gates | Copies of sibling implementation |
| `aram-authd` | Pure-Go LGT carrier DRM/auth handshake emulator injected by `aram-emu` as the raptor `NetBackend` | Live network services, guest execution, UI |
| `aram-test` | Black-box probe orchestration, authorized corpus runs, compatibility deltas, history, and failure triage | Emulator implementation or proprietary inputs |
| `aram-cheat` | Per-title cheat catalogs keyed by input SHA-256, schema, and catalog validation | Game files, dumps, assets, or emulator code |
| `anycall_magichole` | Reverse-engineering notes, scripts, traces, recovered structures, executable reference | Clean-room product implementation claims |

Their Git histories remain independent. `aram-cheat` is data the product reads
at runtime rather than a build input, so a cheat ships without a release and
without pinning a new component revision.

## Planned repositories

These are roadmap boundaries, not repositories to create prematurely:

- `aram-specs`: machine-readable WIPI, carrier, OEM, device, and ABI
  descriptions after the schema is stable enough to have independent users;
- `aram-compatibility`: public or separately distributable compatibility
  results extracted from private `aram-test` reports after the reporting
  schema and privacy review are complete;
- `aram-wipi-sdk`: clean development headers, stubs, examples, and tooling
  after enough WIPI behavior is understood to support third-party programs.

Until those extraction criteria are met, specifications and compatibility
contracts stay with `aram-core` or this repository. Avoid creating empty
repositories whose only content is a future idea.

## Versioning and integration

- Each code repository tags its own semantic version.
- `aram-emu` pins exact core, frontend, and authd revisions for a product
  build.
- Development may use a local Go workspace; CI and releases use reproducible
  tagged or commit-pinned dependencies.
- Contract changes land with a migration document before the integration pin
  advances.
- Release artifacts are produced by integration workflows, not copied into
  Git.
