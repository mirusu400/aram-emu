# Core and frontend integration

`aram-core` and `aram-frontend` build independently. The frontend declares the
small backend interface it needs; the integration adapter satisfies it with a
core machine.

## Dependency rule

```text
aram-frontend  <---  adapter in aram-emu  --->  aram-core
```

The frontend never imports core internals. Core never imports Ebitengine or
native host packages. The adapter may import both and is replaced or extended
when application mode and system mode need different machine factories.

## Open pipeline

Every source entry point converges on the same request:

1. desktop File/Open, drag-and-drop, command line, file association, Android
   intent, or iOS document picker produces a source request;
2. the native host resolves permissions and supplies a path, copied cache
   file, seekable stream, or integration-owned handle;
3. core inspection detects the format and computes a hash without trusting the
   extension;
4. profile resolution selects or requests WIPI, carrier, OEM, device, and title
   settings;
5. the adapter creates the correct application or system machine;
6. frontend state moves from loading to ready or to a structured error.

Android content URIs are not assumed to be normal filesystem paths.

## Implemented application milestone

The application adapter now implements this pipeline for filesystem-backed
desktop inputs. It reports inspecting/loading progress, hashes the source,
lets core resolve the layered compatibility profile, creates the portable
application machine, maps EADS text/BSS/stack, and exposes the resulting core
state and frame to the frontend. Synthetic and private authorized-reference
tests use the same `frontend.OpenRequest` and adapter path.

The portable ARM/Thumb interpreter, public WIPI trampoline boundary, and
profiled EADS service runtime now execute the known reference lifecycle
through bootstrap, setup, start, preload, and a deterministic first visible
frame. This is a hash-keyed title-profile milestone, not a claim of generic
WIPI or multi-title compatibility.

## Command mapping

| Frontend operation | Core/integration responsibility |
|---|---|
| Start, pause, stop, reset | Validate state transition and control machine loop |
| Frame advance, fast-forward | Drive virtual time and presentation cadence |
| Save/load state, rewind | Serialize schema, backend, profile, and input identity |
| Screenshot | Capture guest-native framebuffer without UI scaling |
| Cheat, patch, memory search | Use checked memory/debugger services |
| Compatibility report | Record hashes, profile, versions, replay, and outcome |
| Debug export | Provide bounded core metadata and log/trace tails without guest data |

Frontend commands remain visible when a backend does not support an operation;
the adapter reports capability and reason.

## Cheats

Opening an application wraps the core machine with the `aram-core` cheat
engine, so every later command runs through the wrapper that keeps cheats
serialized with guest execution. Optional reporting interfaces are reached
with `Unwrap`, which is read-only; running or mutating the guest still goes
through the published machine.

The Cheat Manager panel is backed by the `aram-cheat` repository, which stores
one catalog per title named by `ImageInfo.ImageSHA256`, the identity of the
loaded executable image. Keying on the image rather than the input file means
a re-archived package keeps its cheats, since only the container changed.

Each lookup tries the image identity first and the input file hash second, so
an entry published before an image identity was known still resolves. The
Compatibility Report shows both hashes for a reporter to copy. The adapter
resolves a catalog in this order:

1. `ARAM_CHEAT_DIR`, a local checkout, for authoring and offline use;
2. the cache an earlier fetch wrote under the user config directory;
3. `titles/<image sha256>.json` from the database over HTTPS.

Only the last step needs the network, and only the first time a title is seen.
`ARAM_CHEAT_DATABASE` overrides the database base URL. A title with no
published document is reported as an ordinary answer rather than a failure.

The panel publishes one self-applying checkbox per cheat, so toggling a control
runs the `toggle` action immediately. That action receives every toggle's state
and applies the difference against the library, rather than trusting the panel
to say which control moved. Guest memory stays behind this boundary; the
frontend owns only the controls.

A catalog entry may declare itself `default_enabled`, for a repair a title
cannot run without. Those apply as the title loads, so a game whose licence
server is gone comes up working. Each cheat a person turns on or off is
recorded under the user config directory beside the catalog cache, keyed by
image identity, and a recorded choice outranks the default forever after.

Every patch declares the original bytes it replaces, and the engine refuses to
apply one whose expected bytes do not match guest memory. All patches of a
cheat apply as one unit, and a failure rolls the applied patches back.
`TestPublishedCheatsApplyToTheirReferenceTitle` checks a published catalog
against the real image when `ARAM_TEST_DATA` and `ARAM_CHEAT_DIR` are set.

The optional `frontend.DebugExportBackend` contract returns named diagnostic
files. The frontend owns ZIP creation, path redaction, attachment validation,
checksums, and partial-bundle behavior. The integration adapter serializes
core operations while taking a diagnostic snapshot so CPU registers and
runtime trace counters describe one consistent execution boundary.

## Development workflow

During contract development, a local Go workspace can reference sibling
checkouts without publishing unstable module versions. Before a product build:

1. core and frontend pass their own CI;
2. integration pins exact revisions;
3. adapter contract tests run against both application and system stubs;
4. desktop packages and mobile libraries are built;
5. smoke tests open synthetic and authorized reference inputs through the
   ordinary product path.

Private module access credentials belong in CI secrets and never in repository
URLs or files.
