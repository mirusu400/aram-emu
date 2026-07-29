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

Frontend commands remain visible when a backend does not support an operation;
the adapter reports capability and reason.

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
