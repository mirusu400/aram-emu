# Android runtime performance

The Android host supplies output properties and scheduling hints to the shared
frontend. Guest clocks, audio generations, input validation, and saved settings
remain owned by the existing backend and frontend contracts.

## Implementation stages

1. Connect AudioManager output properties to application buffering and diagnostics.
2. Process accepted input earlier while preserving the requested guest speed.
3. Apply performance hints to the actual frame worker and priority to the PCM pump.
4. Verify frontend behavior, Android lifecycle fallback, bindings, lint, and APK builds.

These stages are implemented. Device measurements determine whether a later change
to the guest audio renderer is needed.

## Audio output

Android reports its preferred sample rate and output frames per buffer before the
frontend starts. The host refreshes them on resume and audio device changes.
Missing or invalid properties preserve the portable buffering policy.

With valid properties, the audio latency setting budgets the application queue
and player read-ahead together. Each receives half the budget, rounded up to a
reported output burst. For example, 192 frames at 48 kHz produce a 4 ms burst;
a 60 ms setting selects 32 ms for the queue and 32 ms for the player. OS, Oboe,
and hardware buffering are additional, so this is not a measured total latency.

The PCM supply pump runs every half burst, bounded to 2–8 ms. The portable
fallback remains 4 ms. Paused or closed titles, background hosts, and lost audio
focus stop the pump's ticker. Playback, lifecycle, and focus changes wake it
without waiting for a polling interval. A title that exits on its own has its
final published PCM drained before the pump becomes idle. The output keeps
its existing continuity, generation,
silence smoothing, and queue capacity handling. Stale PCM checks include both
parts of the Android buffering budget rather than only the smaller queue target.

Debug audio telemetry includes `reported_device_rate`, `reported_burst_frames`,
`player_buffer_ms`, and `pump_interval_us`. Reported properties describe host
defaults, not the stream format negotiated by Oboe. The audio trace aggregates
all frame work over a one-second window beside queue underruns and fill. Each
entry includes the frame count, total duration, and longest frame in that window.
Pausing, closing, generation changes, and debug export preserve incomplete
windows too, so a short stall between regular entries is retained.

## Input and frame work

An input transition is eligible only after the backend accepts it. If less than
one guest quantum is available, the pacer may borrow one quantum to process the
input sooner. The accumulator records a negative balance and later ticks repay
it. Repeated input cannot borrow again while that balance is negative. Pausing
clears the pending input request. Input also interrupts the worker's optional
3 ms UI priority rest, with wake requests coalesced into one channel entry.

This improves scheduling after an input has been sampled and accepted; it does
not bypass Ebitengine's touch/gamepad sampling or execute extra unbudgeted frames.

The frame worker reports actual RunFrame duration, excluding its UI rest, on its
locked OS thread. Its target follows the effective pacing speed. Android API 31+
uses a PerformanceHintManager session for that worker. Pausing or destroying the
Activity closes the session; resuming opens a new session at the next work item.
Unavailable or failing hints fall back without retrying every frame.

The default worker requests display priority. UI priority mode requests normal
priority. The separate lightweight PCM pump requests audio priority. Priority
failures leave emulation working. The debug pacing report includes work count,
mean and maximum frame work duration, and the count of borrowed input quanta.

## Verification

Frontend regression tests cover burst alignment, unavailable output properties,
combined buffering during stale PCM checks, input debt repayment, sustained rapid
input, coalesced wake requests, work targets at different speeds, and bounded
audio/frame trace aggregation, incomplete window retention, event-driven pump
idle/resume, and final PCM after guest exit. Android unit tests cover session reuse, target
changes, pause/resume, destruction, and unavailable or failing hints.

Local checks:

```powershell
go test github.com/mirusu400/aram-frontend/frontend ./integration ./systemintegration
gradle --no-daemon -p android :app:testGithubDebugUnitTest :app:lintGithubDebug :app:assembleGithubDebug
```

Rebuild the mobile AAR before compiling Android: the host uses the new
`PerformanceHost` and `ConfigureAudioOutput` bindings. Play builds also require
their AAR to be rebuilt with the existing SelfUpdateDisabled flag.

On a device, record the same title and scene with its CPU backend, audio settings,
display sync, and output route. Compare frame work, underruns, queue fill,
buffered player frames, perceived input latency, and sustained temperature.
No device improvement percentages are claimed from unit tests or builds.
