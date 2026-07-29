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
| `aram-test` | Black-box probe orchestration, authorized corpus runs, compatibility deltas, history, and failure triage | Emulator implementation or proprietary inputs |
| `anycall_magichole` | Reverse-engineering notes, scripts, traces, recovered structures, executable reference | Clean-room product implementation claims |

All five repositories are private. Their Git histories remain independent.

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
- `aram-emu` pins exact core and frontend revisions for a product build.
- Development may use a local Go workspace; CI and releases use reproducible
  tagged or commit-pinned dependencies.
- Contract changes land with a migration document before the integration pin
  advances.
- Release artifacts are produced by integration workflows, not copied into
  Git.
