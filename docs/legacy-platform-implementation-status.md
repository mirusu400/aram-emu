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

A memory-only probe of the standalone inner JAR with SHA-256
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
ZIP. The failure remains the missing `mmpp/media/MediaPlayer` class.
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

Final isolated-workspace verification on 2026-09-11:

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
