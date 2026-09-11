# Legacy platform implementation evidence

This is the implementation follow-up to
[the platform support plan](legacy-platform-support-plan.md). Recognition,
loading, guest entry execution, frame publication, booting, playability and
completion remain different milestones. This change is not a claim that the
P0–P7 programme or any entire commercial platform is complete.

## Ownership and contracts

- `aram-core/loader/j2me` validates standalone MIDlet JARs and local ZIP
  distributions with one JAD/JAR pair. Parsing has archive/member/aggregate,
  descriptor and class validation bounds. Descriptor URLs never trigger a
  download. Standalone JAD document resolution is not implemented.
- The existing Java VM/host is reused through a common Java application input.
  Generic and explicitly selected LGT profiles are separate from SKT. SKT-only
  native installation is not silently enabled for generic/LGT MIDlets. Mixed
  compatibility installers retain implemented standard MIDP key/font/drawing
  methods while excluding OEM extensions. JAD/manifest application properties
  remain visible through MIDlet.getAppProperty, not System.getProperty.
- Existing KTF, Raptor, SKVM, native WIPI, GNEX and Android detection retain
  precedence over generic Java fallback. Validated BREW distributions are not
  executed as nested MIDlets.
- `aram-core/loader/brew` validates the observed MIF envelope and reports opaque
  MOD members. MIF/MOD basenames are not assumed to pair. Signature presence is
  metadata only, not signature verification or protection removal.
- `aram-core/loader/gnex` recognizes bounded standalone SGS headers in addition
  to paired packages. Its undecoded body is not sent to the Java or ARM VM.
- `application.UnsupportedPlatformError` preserves validated BREW/GVM kind and
  recognition identity through the ordinary integration path. The profiles
  `brew-container-v1/unknown/generic` and `gvm-container-v1/skt/generic` version
  the recognition schema, not a guest SDK, ABI or executable save-state format.
  No runnable machine or save-state capability is supplied for these inputs.
- Product and frontend retain normal open/control workflows. Missing platform
  capabilities remain unavailable rather than being replaced by a title launcher.

## Read-only corpus baseline

The frozen pre-change product probe was built from the isolated starting
revisions before implementation edits. The inventory contains 483 distinct
SHA-256 inputs: 475 ZIPs, one ALZ and seven other files. The original probe
reported `unsupported_format` for all 483 at 32,768 slices and a 20-second
per-input timeout. The ZIP total is not a count of unique, complete games.

Static BREW evidence covers 359 MOD and 362 MIF entries. MOD files are not ELF
or PE images. ARM-like startup words do not establish a load address, entry
contract, relocation scheme or BREW object ABI. All inspected MIFs satisfy the
observed bounded envelope, but records and MIF-to-MOD associations remain
undecoded. Three distributions contain MIF metadata without a MOD.

The Java dependency audit extracts constant-pool class and exact method
references and subtracts classes bundled in each JAR. This is a static
external-dependency inventory, not proof that every reference executes or that
an API behaves correctly. The audit parsed 105 JARs and 1,097 classes, finding
81 distinct external classes and 394 external member references across the
corpus. All 105 JARs reference `mmpp/media/MediaPlayer`. No fabricated
success-returning API was added to suppress those dependencies.

Before the subsequent MMPP adapters below, a memory-only probe of the standalone
inner JAR with SHA-256
`ffd8c6e82ce6afabb270886a162d28fbef3f88332ee2ff40e139abfd369f4a06`
(142,782 bytes), using the explicit LGT profile, loads and executes 2,200 Java
instructions. It then fails allocating `mmpp/media/MediaPlayer`, before any
method invocation, with no current display and no published frame. This is
**loads plus entry execution**, not first-screen or input-response evidence.
The outer ZIP with SHA-256
`6572ae8347b894504b893ca7ad3a51fc021c9ab98abc49bed4761e2c501ca733`
is a different input. After the evidence-based archive-alias fix, the ordinary
product probe also reports `j2me`, explicit LGT profile, `guest_fault` at
`loads`, 2,200 instructions, no display and zero presentations for that exact
ZIP. At that stage the failure was the missing `mmpp/media/MediaPlayer` class.
Archive aliases with unrelated filenames require exactly one JAD/JAR pair,
an absolute HTTP(S) metadata URL, matching declared JAR size, all three
nonempty matching name/version/vendor fields, and an identical nonempty main
class in JAD and manifest. Relative URL resolution is not relaxed, and URLs
are never downloaded. No proprietary JAR was extracted to disk for the
separate inner-JAR probe.

Only hashes, normalized classifications and aggregate evidence belong in
public reports. Inputs remain in place, with no private archive, resource,
absolute path or raw dump committed.

## Completion boundaries

| Plan phase | Implemented scope | Remaining acceptance evidence |
|---|---|---|
| P0 | Frozen baseline, bounded inventory, exact external Java dependency audit and final comparison | No comparison regression; scope differences recorded below |
| P1 | Synthetic-tested bounded J2ME loader and collision handling | Ordered synthetic gate passes; full private reference gate remains blocked below |
| P2 | Common Java host and normal product path with independent identities | A selected original hash must demonstrate expected first screen and input response |
| P3 | Native-policy separation and synthetic lifecycle/state coverage | Multiple real API clusters, Korean rendering, timing, audio and RMS semantics |
| P4 | BREW container/MIF envelope recognition only | Verified MOD load/entry/relocation and object/event ABI, then bootstrap tests |
| P5 | Not implemented | Resource/service adapters and real screen/input/storage evidence |
| P6 | Existing SGS header evidence hardened | Verified version-specific opcode, stack, control-flow and resource semantics |
| P7 | Not implemented | Evidence-based VM, service/state integration and real input milestones |

External `.db`/`.idx` import is not treated as empty successful storage. Unknown
BREW protection, missing modules, malformed data and undecoded GVM execution
remain explicit unsupported/error cases. New synthetic Java success does not
prove compatibility for every original LGT input.

## Verification record

The configured full private `all` gate is **not passed**: the supplied legacy
corpus has no valid KTF WIPI or Raptor package for the existing mandatory native
reference tests. Those tests remain unchanged and fail on missing input. No
private input was invented, copied from another source, or silently skipped to
make that gate green. Black-box corpus results and this reference-test blocker
must be reported separately.

The ordinary Windows product CLI path showed the synthetic input's window
title and recent identity, and its exact spawned process was stopped. Native
Ctrl+O dialog/filter visibility was checked. Actual native chooser selection
remains unverified because foreground/control automation was unavailable.

The SDK gate exposed a pre-existing headless audio mismatch: both the frozen
and changed probes produced 44.1 kHz mono for the same synthetic media input,
while the existing gate requires stereo. The probe now explicitly requests
44.1 kHz stereo from the backend converter. The expected format was not
weakened. The rerun produced stereo with zero discontinuities, invalid chunks
or dropped samples, and all ten SDK examples passed.

The optional race run could not execute because CGO is disabled and no host C
compiler is installed. Required pure-Go tests are separate from that check.

The 32,768-slice concurrent gate also exposed repeated copying and hashing of
unchanged Java framebuffers. The host now uses the existing presentation commit
API and caches independently verified diagnostics only across clean consecutive
presentations. Changed pixels are rehashed, callers cannot mutate the cache, and
reset/state restoration invalidates it. A local single-input reproduction fell
from 14,692 ms to 77 ms for JAR and 84 ms for ZIP, retaining all 32,768
presentations, four guest instructions and the same full framebuffer hash.
Deterministic tests cover allocation bounds, timers, queued input, virtual time,
audio and byte-identical state replay. No timeout, slice count or expected
milestone was weakened.

Initial isolated-workspace verification on 2026-09-11, before the MMPP continuation:

| Check | Observed result |
|---|---|
| Core, frontend and integration `go test -count=1 ./...`, `go vet ./...`, formatting and diff checks | Pass |
| Runner unit suite | 76 pass |
| Synthetic `all --force --jobs 4 --slices 4096` | Exit 0, 13/13 expectations pass, one configured reference input runs, no regression, triage empty |
| Authorized `all --force --jobs 4 --slices 32768 --timeout 20` | Exit 1 solely from mandatory KTF/Raptor reference tests finding no valid input; not a passed full gate |
| Authorized black-box phase | All 475 ZIPs probed, 13/13 synthetic expectations pass, no timeouts, no compatibility regression |
| SDK examples | 10 pass, 0 fail |
| Windows `aram`, `aram-probe`, `aram-frontend` builds | Pass |
| Pure-Go core Android/arm64, Linux/amd64 and Darwin/arm64 builds | Pass |
| Official `ebitenmobile bind -target android` | Pass, AAR produced |
| Generated report/delta/triage privacy checks | No private root/input path markers; source changes contain no private input bytes |

Of the 475 private ZIP results, 447 remain unsupported (358 BREW execution,
nine GVM execution and 80 external savedata), 24 have precise validation
failures, and four reach Java entry execution before a guest fault. None proves
a commercial first screen. The seven remaining triage clusters are recorded
without raw private diagnostics.

Comparison against the frozen 483-input baseline reports four improved,
24 changed, 447 unchanged and **zero regressed**. The comparator's improved
category means guest entry progress here, not playability. Fourteen new rows
are synthetic/reference inputs. Eight removed rows are the ALZ and seven other
files included in P0 inventory but outside the ZIP black-box discovery scope,
not successful executions or missing configured roots. Product dependency
revisions are pinned in `product-components.json` in the coordinated commit.

Post-submission integration found concurrent upstream KTF and Android libretro
changes. These were merged, not overwritten. The probe audio conflict retained
the explicit stereo helper, and the core pin includes both sparse KTF support
and this Java work. The complete ordered local gate was rerun after the merge:
all public/synthetic/build/binder checks still pass, and the same missing-input
private reference failures remain. This merge does not claim a new libretro
device smoke test or remove any upstream emulator entry path.

## Explicit LGT MMPP continuation

The later continuation uses publicly mirrored Korean MMPP Javadoc, not handset
implementation code. The third-party archive does not identify an exact handset
or SDK build. Documented facts and emulator choices are separated in core's
`docs/lgt-mmpp.md` and `docs/lgt-mathfp.md`. No BREW module bootstrap or GVM
opcode contract was established by the additional public research. P4–P7
execution requirements therefore remain unimplemented, not silently satisfied.

The explicit LGT policy adds bounded, service-backed MediaPlayer source and
playback operations, timed BackLight on/off, and an integer-based MathFP subset.
Generic J2ME and SKT do not receive these MMPP classes. Volume String semantics,
backlight colors, external savedata import and unimplemented mathematical APIs
remain unsupported. The LGT native-policy digest rejects pre-adapter LGT states
instead of silently accepting a changed host contract.

The ordinary Windows product accepts `--profile <profile-id>` for one local
initial input. The override does not leak into later File/Open requests or a
different input selected for updater relaunch. No title-specific launcher or
filename-based carrier inference is introduced.

| Changed contract | Concrete verification |
|---|---|
| LGT policy and state identity | Host configuration tests, exact registry isolation, atomic cross-policy state rejection |
| MediaPlayer semantics | Nonzero decoded PCM, pause/resume position, loop count, source slicing and transactional replacement, deterministic replay |
| Shared media isolation | `TestMediaPrepareAndStoppedDestroyPreserveOutput` and two-player source tests preserve queued PCM and output revision; active destruction is still rejected |
| BackLight timing | Virtual deadline expiry, zero-duration indefinite state, explicit off, restored deadline and invalid-call state preservation |
| MathFP subset | Native invocation and guest bytecode/static constants, rounding boundaries, overflow and guest exception catches |
| GraphicsX identity | Three ordinary graphics producers, exact host inheritance, unrelated-type rejection, generic/SKT isolation and save-state identity replay |
| Ordinary profile CLI | Parser error cases, scoped OpenRequest and relaunch behavior tests |
| Recurring LGT product path | Same synthetic bytes under explicit LGT and generic profiles, exact initial and post-key RGBA hashes and two delivered input events |

The new synthetic fixture SHA-256 is
`5c4b58b68d7da2fd09525b784bad8a3a0214b47ccc95c702570ff1cb156dea8f`.
The frozen ordinary probe faults on missing MediaPlayer at four instructions.
The changed explicit-LGT path publishes the expected frame, while generic J2ME
still rejects the same MMPP dependency. Audio sample assertions are owning core
tests, not inferred from framebuffer success or the runner's telemetry.

Independent review reproduced a source-validation bug: configuring a second
player discarded the first player's 882 queued samples through temporary
Play/Stop calls. The owning runtime now prepares a stopped clip without starting
playback, and removing a stopped clip does not invalidate unrelated queued PCM.
New source, replacement and rejected source cases preserve the first player's
output. This is a tested shared-service fix, not a snapshot rollback workaround.

An ordinary explicit-LGT probe of original ZIP
`6572ae8347b894504b893ca7ad3a51fc021c9ab98abc49bed4761e2c501ca733`
advances from the historical 2,200-instruction missing-class fault to a first
published frame at 2,524 instructions. The frame hash is
`cafe07a981e81aae7ab1f7f7cc38b0349a8ad3d635d3cfef2352c46eeccc5ad0`.
This observation alone is not an expected-screen, input-response or playability
claim. An initial input probe exposed a separate Graphics-to-GraphicsX cast
failure at 13,733 instructions. The public GraphicsX documentation explicitly
states that all LGT Graphics objects are GraphicsX instances. Screen, mutable
Image and GameCanvas producers now use that exact LGT-only identity and its
documented inheritance. Java type checks are unchanged.

The final ordinary probe retains the same initial hash and remains running after
128 time-only slices (13,376 instructions). With two OK input events it passes
the original cast and reaches 13,743 instructions, then faults on unimplemented
`GraphicsX.setAlpha(I)V`. The displayed hash remains unchanged. Input delivery is
verified, but expected input response is not. P2 original-screen/input acceptance
therefore remains incomplete. This cycle does not advertise GraphicsX extension
methods merely because its object identity is implemented.

A separate original ZIP,
`8efbf9644e6b16bf98ec8a33867424588ba023ebaf5afee76e6a50af285a6484`,
was reprobed in place through the ordinary explicit-LGT product probe with fresh
state. Its missing `MathFP.parseFP(I)I` failure at 690 instructions is resolved.
Execution now reaches 1,310 instructions and stops at the explicitly unsupported
`MediaPlayer.setVolumeLevel` contract. It remains `guest_fault` at `loads`, with
no display, no presentations and no valid frame. This is API-cluster progress,
not a second original first-screen success.

### Frozen-source continuation verification

The complete ordered loop was rerun after the GraphicsX and media-isolation
fixes, with no behavioral source edits during the final gate:

| Check | Final observed result |
|---|---|
| Core, runner, frontend, integration in required order | Go tests/vet/diff checks pass; runner 83 tests pass |
| Synthetic `all`, 4,096 slices, jobs 4 | Exit 0; 15/15 expectations pass; configured reference runs; no regression; triage empty |
| Actual corpus `all`, 32,768 slices, 20 seconds, jobs 4 | Exit 1 from six existing mandatory tests finding no valid KTF/Raptor package; zero unexpected skips; not a passed full private gate |
| Actual black-box coverage | All 475 original ZIPs, 15 synthetic cases and one reference input run; no timeout; synthetic 15/15 pass |
| Same-scope continuation comparison | All 489 existing rows unchanged, two added profile-specific synthetic rows, zero regressions and zero removed rows |
| SDK examples | 10 pass, 0 fail |
| Windows product builds | `aram`, `aram-probe`, `aram-frontend` pass |
| Ordinary Windows original-input CLI smoke | Visible frame, exact original SHA and explicit LGT profile verified through normal UI; spawned PID 41144 stopped, protocol registration restored, temporary screenshot removed |
| Pure-Go core portability | Android/arm64, Linux/amd64, Darwin/arm64 pass |
| Official Android frontend binder | Pass; AAR produced |
| Public report privacy | Seven Markdown and nine JSON aggregate/delta/triage artifacts contain no checked private root/input-path markers; owned source review contains no private inputs |

The full corpus retains default profile selection. It does not silently infer
LGT from filenames or apply a synthetic-only profile override to private cases.
Therefore the separately measured explicit-LGT improvements above do not change
those 489 default-profile comparison rows. The seven existing private triage
clusters remain visible, headed by unsupported BREW execution and external
savedata import. Passing synthetic contracts and builds does not close the
missing-reference, original input-response or P4–P7 acceptance requirements.

The Windows smoke exposes the same unsupported `GraphicsX.setAlpha(I)V`
boundary. It verifies ordinary initialization and visible identity, not a
correctly rendered reference screen, successful gameplay or automated native
File/Open chooser selection.

### Concurrent upstream follow-up

The PR component pin conflicted with newly published upstream revisions. The
coordinated branches merge, rather than overwrite, the KTF initial native
framebuffer and file-read exception fixes, Raptor direct-input HAL support,
shared-media loop/effect isolation tests, and SDK default-format correction.
The product pin is updated to a core commit containing both lines of work.

Upstream SDK commit `3b4fc6e` corrects its format assertion to the ordinary
product's existing 44.1 kHz **mono** default. This agrees with the frontend's
default-channel regression test and the previously frozen probe output. The
earlier forced-stereo workaround described in the historical record is therefore
superseded: the probe now explicitly requests the same mono default. Its owning
test first fails with the old stereo setting, then passes with mono for both mix
policies. The upstream SDK assertions, diagnostic zero requirements, and existing
stereo-chunk validation tests are retained, not loosened to accept either format.

The complete post-merge ordered loop passes public tests/vet, all 83 runner
tests, synthetic 15/15, SDK 10/10, Windows builds, core portability and official
Android binding. The SDK audio-player and media-suite outputs are observed at
44.1 kHz mono with the existing continuity checks passing. Full private `all`
still exits 1 from the same six missing KTF/Raptor-input tests, with no unexpected
skips. All 491 pre/post-merge black-box rows are unchanged, with zero regressions
or removals, and the seven private triage clusters remain visible.

A fresh ordinary explicit-LGT original-input rerun after the merge reproduces
the same 2,524 / 13,376 / 13,743 instruction milestones and identical framebuffer
hash. The remaining `GraphicsX.setAlpha(I)V` input boundary is unchanged.

### GraphicsX alpha follow-through

This subsequent change supersedes only the `setAlpha` boundary above, not the
historical measurements or the unfinished P0-P7 requirements. The mirrored
GraphicsX Javadoc specifies alpha 0..256, default 256 and
`IllegalArgumentException` outside that range. Core adds the exact LGT-only
native and default field, per-Graphics state, and a shared 256-scale compositor.
Blend rounding is an explicit emulator choice, not verified handset behavior.
Old graphics snapshots retain opaque defaults, while explicit transparent state
round-trips. Drawing temporarily applies and restores object-local alpha so
multiple contexts sharing one image do not leak it. Existing rounded-rectangle
geometry limitations are not claimed fixed by alpha support.

The independent runner retains all original 15 fixture payloads, profiles and
expectations and adds two identical alpha payloads with explicit LGT/generic
profiles. Guest bytecode checks default/0/128/256 alpha and catches -1/257 errors
before drawing again. The independent RGBA oracle expects white-on-black
midpoint 128 and a source-alpha test pixel 64. Generic execution is rejected at
the first GraphicsX cast with `ClassCastException`, not mislabeled as a missing
native. Its object allocation number is not an acceptance contract. All 89
runner unit tests and all 17 ordinary-probe synthetic cases pass in the focused
loop. Final ordered workspace gates are recorded separately below when run.

The same untouched original SHA
`6572ae8347b894504b893ca7ad3a51fc021c9ab98abc49bed4761e2c501ca733`
under explicit `j2me-1.0/lgt/generic` was rerun through the rebuilt ordinary
product probe with fresh state:

| Observation | Result |
|---|---|
| Initial frame | Running, 2,524 instructions, previous frame SHA unchanged |
| Time only, 128 post-frame slices | Running, 13,376 instructions, previous frame SHA unchanged |
| Two OK press/release events, 128 post-frame slices | Running, 16,299 instructions, 165 presentations and a changed valid frame |
| Two OK events, 4,096 post-frame slices | Later MediaPlayer fault at 43,907 instructions and 576 presentations |
| Four events from two OK presses, longer release window | Later MediaPlayer fault at 43,965 instructions and 576 presentations |

The short input-response frame SHA is
`15379b97ec3f04cb7c2ed55964055a96a5cd338b29edaad27eb2ec3fddb854f3`.
This replaces the prior 13,743-instruction `setAlpha` fault with a concrete
input-triggered frame change. Longer observation still fails, so this is not
expected-reference-screen verification, sustained gameplay or playability.
Private inputs remain in place, and observations retain no resource bytes,
input paths or raw diagnostics.

The frozen-source ordered loop completed after this change: all core,
frontend and product Go tests/vet/diff checks, 89 runner units, synthetic
`all` 17/17, SDK examples 10/10, Windows product builds, pure-Go core
Android/Linux/macOS builds and the official Android binder pass. The configured
private `all` still exits 1 from the same six mandatory KTF/Raptor tests finding
no valid supplied input, with zero unexpected skips. Its black-box phase runs
475 original ZIPs, 17 synthetic cases and one reference. The same-scope
comparison has 491 unchanged rows and two new alpha profile rows, with no
regressions or removals. Seven existing private triage clusters remain, headed
by 358 unsupported BREW execution cases. Sixteen public aggregate/delta/triage
artifacts have no checked private path/root/key markers.

A fresh ordinary Windows `aram --profile ...` launch of the same original,
with isolated settings, visibly renders an intro with graphics, Korean text,
SKIP and Running status. After an attempted foreground Enter press/release,
the UI exposes `MediaPlayer.stop: no source; handset error unspecified`.
Automation timing does not establish that this attempted GUI input caused the
fault or successfully reached the guest, so the GUI check proves visible
initialization and truthful diagnostics, not an OK-response or gameplay claim.
The ordinary probe separately establishes the short input response above.
The exact spawned application PID 49340 and conhost child 23784 were verified
stopped; temporary captures/binary/settings were removed, protocol registration
matches its prelaunch backup, and the original input hash is unchanged.
