# Compatibility evidence

ARAM compatibility is tracked per input hash, runtime/profile selection, and
component revision. Anecdotal platform-wide labels are not accepted.

Each record contains at least:

```yaml
title: Synthetic example
sha256: 0000000000000000000000000000000000000000000000000000000000000000
format: eads
mode: application
wipi: "2.x"
carrier: ktf
manufacturer: samsung
device: example
core_revision: 0000000
frontend_revision: 0000000
integration_revision: 0000000
cpu_backend: interpreter
result: boots
input_replay: traces/synthetic-example.json
notes: No proprietary data
```

Result levels are:

1. `recognized`
2. `loads`
3. `boots`
4. `menu`
5. `playable`
6. `complete`

A higher level requires reproducible evidence for all lower levels. Records
also distinguish crashes, hangs, graphical defects, audio defects, input
defects, performance issues, and unimplemented APIs.

Firmware milestones use a separate vocabulary such as `snapshot-entry`,
`amss-entry`, `ui-visible`, and `cold-boot`. Application-mode success must not
be reported as full firmware support.

Compatibility data never includes the game, firmware, keys, extracted assets,
or private user paths. Public distribution of reports waits for a privacy and
licensing review.

## Automated suite

The sibling `aram-test` repository owns the automated compatibility suite.
It follows the ordinary integration adapter and core application machine
through this repository's headless `cmd/aram-probe` executable, without
maintaining a second loader or emulator implementation.

`ARAM_TEST_DATA` and `ARAM_REFERENCE_REPO` opt private inputs into a local
`aram-test` run without copying or modifying them. That repository owns
generated fixtures, per-input caches, reports, cross-revision deltas, failure
clustering, and scheduled synthetic CI. Private corpora remain excluded from
hosted CI.
