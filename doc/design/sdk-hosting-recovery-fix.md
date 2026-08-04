# SDK Hosting Recovery Bug: Fix Plan

## Summary

Go SDK hosted services do not recover after their bind (service) session is invalidated. This is
triggered by the controller upgrade from 1.6 to 2.0: the 2.0 controller deletes all pre-2.0 service
sessions (a known, correct compatibility step, pre-2.0 sessions are not understood by 2.0), and the
routers restart during the upgrade. After that, a Go SDK host that held its connection through the
event never re-creates its bind session. It keeps re-binding the stale, now-deleted session, so
terminator creation fails with "invalid session" forever, and that host's traffic stays dead until the
process is restarted.

The C SDK (ziti-edge-tunnel) and edge-router-tunnelers recover correctly in the same scenario. A full
restart of the Go app also fixes it (a fresh listener manager creates a valid session). So the controller
behavior is correct; the gap is in the Go SDK's hosting-recovery path.

## Evidence (from the upgrade-test live run)

- Controller: `create.terminator` -> `loadFromBolt: invalid session` (create_terminator_v2.go). The
  router presents the SDK's bind-session token; the controller cannot find it in bolt because it is the
  stale/deleted session.
- Go SDK host log: `establishing bind session` = 0, `successfully created` = 0, `failure creating` = 0.
  So `createSession` is never called, the SDK is not re-creating the session at all.
- Go SDK host log: ~21k `timed out sending listener start-over event`, ~36k `invalid session`.
- Reproduced on both SDK versions, so it is not version-specific:
  - v2.0.0-pre2 (loop4 host): tight `invalid session` loop.
  - v1.2.4 (ziti-tunnel host): ~135k `established listener` churn with no terminator ever created.
- Not the api-session: the recovered ZET hosts hold the same pre-upgrade legacy api-session as the
  stuck Go hosts.
- A full app restart recovers immediately (fresh listener manager creates a valid session).

## Root cause

The Go SDK `listenerManager` hosting-recovery path is unreliable. On a hosting error the router sends
`invalid session` with retry hint `RetryStartOver`. The SDK handles this via `NotifyStartOver`
(`ziti/ziti.go`), which schedules, after a 5s delay, a best-effort send of a `listenerStartOverEvent`
onto a single `eventChan` with a 1s send timeout. Under a flood of per-connection failures the channel
is swamped and the start-over events are dropped (the "timed out sending listener start-over event"
messages). Since `listenerStartOverEvent.handle` is what nulls `mgr.session` and forces a re-create, the
manager never re-creates the session. It just keeps re-binding the stale `mgr.session`, which the
controller rejects. The result is an infinite "invalid session" loop with `createSession` never invoked.

## Why the recovery event is dropped

`eventChan` is buffered at only 3 (`make(chan listenerEvent, 3)`) and is shared by all listener events,
drained one at a time by the single `run()` select loop. During the failure churn the start-over event
competes with a high-volume stream on that tiny channel:

- `listenerEstablishedEvent` on every bind attempt (the `makeMoreListeners` 1s ticker plus router
  reconnects keep re-binding; each local bind success fires one, this is the ~135k `established
  listener` churn seen on the 1.2.4 host).
- `listenSuccessEvent` and `routerConnectionListenFailedEvent` per router listen attempt.
- `getSessionEvent` for every `GetSession` call.
- other `listenerStartOverEvent`s, since every failing hosted connection fires its own (self-flood).

The run loop also stalls in its handlers (`Authenticate`/`createSessionWithBackoff` backoff,
`makeMoreListeners`), so it drains slowly. The start-over is the only event that triggers recovery, yet
it is the most disadvantaged: delayed 5s, best-effort with a 1s send timeout, into a 3-slot channel that
is saturated. So it is dropped every time and `createSession` never runs.

## Fix

Route recovery through a reliable atomic flag on the `listenerManager`, checked at the top of the run
loop, instead of the droppable event. Keep all `mgr.session` mutation inside the run loop (do not
atomic-ify or clear `session` from another goroutine, it is read throughout the loop and would race /
nil-deref).

- Add `needsStartOver atomic.Bool` (the loop trigger) plus a schedule guard so at most one start-over is
  pending at a time. The guard coalesces the per-connection flood: N failing connections produce one
  recovery, not N competing events.
- `NotifyStartOver`: after a randomized delay between 0.5s and 5s (replacing the fixed 5s), set the flag
  and do a best-effort, non-blocking wake of the run loop. The randomized delay both lets the
  controller/router settle and de-correlates restart-hosting across many hosts so they do not hit the
  controller in lockstep (thundering herd).
- Run loop, at the top of each iteration:
  `if mgr.needsStartOver.CompareAndSwap(true, false) { mgr.session = nil; mgr.lastSessionRefresh = {};
  mgr.context.Authenticate(); mgr.refreshSession() }`.

Properties: reliable (no channel/timeout dependency, cannot be dropped), coalesced (one recovery per
window regardless of how many connections fail), and race-free (`session` only mutated in the loop).
Recovery latency is bounded by the loop wake, at worst the 1s ticker when idle, much faster under churn.

Caveat to verify: ensure the run loop's handlers are bounded so a pending start-over is not starved
while the loop sits in a long `createSessionWithBackoff`/`makeMoreListeners` call.

The bar for done: a Go SDK host whose bind session is invalidated (controller upgrade + router restart)
re-creates its session and re-establishes its terminator without a process restart.

## Branch and release strategy

The bug is pre-existing (it reproduces on v1.2.4), so fix on the stable line and forward-port rather
than fixing only on the volatile pre-2.0 branch.

1. Create a `release-v1.9.x` branch in sdk-golang off the latest v1.9.x tag (`v1.9.0`).
2. First confirm the bug reproduces against v1.9.0 (the recovery path may differ slightly from pre2 and
   v1.2.4; verify the same "start-over events dropped / session not re-created" behavior).
3. Apply the fix on `release-v1.9.x` and cut a patch release (e.g. v1.9.1).
4. Forward-port the fix to `main` (the 2.0 line) so the 2.0.0-pre releases carry it.

## Validation (via the upgrade-test model)

1. Rebuild the harness, loop4, and ziti-tunnel against the fixed SDK.
2. Fresh `up`, then upgrade the controller and then the routers.
3. Confirm every Go SDK host re-establishes its terminator without a manual restart: `loop-sdk` and
   `loop-ziti-tunnel` terminators return, and the `invalid session` / `timed out start-over` loop does
   not occur.
4. Confirm parity across flavors: SDK, ERT, ZET, and ziti-tunnel all recover after the upgrade.

## Related test-harness follow-ups (separate from the SDK fix)

- Bump the harness and loop4 pinned sdk-golang from `v2.0.0-pre2` to the fixed version once available.
- Version-aware restart/reset: a plain fablab `restart` reverts a component to `fromVersion`, and a
  1.6 controller cannot open a 2.0-migrated DB (`Unsupported edge datastore version`), it crashes. The
  upgrade and reset actions must pin the component's current version rather than relying on `restart`.
- "Is it stable" gate hardening: after a disruptive step, wait for the churn to settle and validate that
  all expected terminators are present (and circuits healthy) before entering the strict stability
  window. A few quick clean scenario runs are not sufficient, SDK hosts can take minutes to
  re-establish, so the settle must be terminator-aware.
- Keep the `RemoteController` close-handler race fix already applied (conditional remove so a
  reconnecting sim is not evicted).
- Deferred: the SDK OIDC-staleness issue (`versionOnce` is a `sync.Once` that is never reset, so a
  long-lived context never re-detects OIDC after a controller upgrade). This governs auth mode, not the
  hosting-recovery failure above, and is lower priority. Note it is not what caused the hosting loop:
  the stuck hosts and the recovered ZET hosts all held the same legacy api-session.

## Open items

- Confirmed: v1.9.0 (tag ff096c4) has byte-for-byte identical `NotifyStartOver` and
  `listenerStartOverEvent.handle` code as v2.0.0-pre2, so the bug and fix apply to the 1.9.x line. The
  current main/2.0 line also carries the same code, so the forward-port is the same change. Only a
  runtime repro on v1.9.0 remains (a formality given the identical code).
- Decide between coalesce-and-deliver vs inline invalid-session handling for the fix.
