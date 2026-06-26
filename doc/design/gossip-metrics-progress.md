# Gossip Links + Link Metrics: Integration Progress

Status snapshot for resuming after a context reset. The design is in
`gossip-metrics.md` (link metrics over gossip) and `gossip.md` /
`gossip-sequences.md` (link-state gossip). This file tracks where the
*implementation and branch integration* stand.

## Current objective

Integrate the unreleased `gossip-links` feature work onto current `main`
(which has moved to **channel/v5** and is 30 commits ahead of where gossip-links
branched) and onto the new **`servermetrics`** package, then split it into a
small stack of PRs.

## Branch / PR state

- **`main`** (`origin`, `f7b7fb30f`): channel/v5. Has no gossip code at all.
- **`metrics-msg-in-ziti`** — PR **#4037** (open). Moves the metrics wire format
  (`MetricsMessage`) + reporting/usage subsystem into ziti's
  `common/servermetrics` package (`+ metrics_pb`), wrapping `openziti/metrics`
  v1 for plain metric collection. Off fresh main, channel/v5, green
  (build/vet/tests pass; wire-compat test included). `openziti/metrics` left
  untouched (no sdk-golang cascade).
- **`gossip-links`** — the original feature branch (stale main base, channel/v4),
  46 commits, pushed to `origin` at `961c91d8f`. This is the history archive; we
  are NOT preserving its granular history. `961c91d8f` includes the broadcast
  race fix (below).
- **`gossip-links-v5`** — NEW integration branch off `metrics-msg-in-ziti`.
  A `git merge --squash gossip-links` is **in progress in the working tree**
  (uncommitted, conflict markers present). Do not branch-switch away without
  stashing/committing or the in-progress merge is disrupted.
- **`openziti/metrics` repo**, branch `link-latency-in-gossip` (`8f9ff84`): added
  `linkLatencyInGossip` to the OLD external proto. Now **moot** — Phase 5 will use
  ziti's `servermetrics/metrics_pb` instead. The metrics `/v2` collection-only
  slim-down is a separate future task (coordinate with sdk-golang v2).

## The bug that crashed the soak (fixed)

Soak `validateLinks` failed because ctrl1's process died. The dmesg OOM was
**old** (host up 7 days). The real crash (in `ctrl1.log`):
`fatal error: concurrent map iteration and map write` in
`channel.marshalHeaders`. Root cause: `RaftMeshAdapter.Broadcast` handed one
shared `*channel.Message` to every peer channel; each channel's heartbeater (a Tx
transform handler) writes a header into the message at send time, racing another
peer's marshal iteration of the same `Headers` map. Gossip itself was healthy
(0 errors, 79,800 links converged).

Fix: `cloneForPeer` in `controller/gossip/mesh.go` gives each peer its own message
(own Headers map, shared read-only body) + a regression test
(`Test_cloneForPeer_independentHeaders`). Committed on `gossip-links`
(`961c91d8f`) and carried into the `gossip-links-v5` squash-merge. NOT needed on
main (gossip doesn't exist there; raft mesh uses point-to-point sends).

## Planned PR split (target shape of gossip-links-v5)

1. **Infra commits, individual + PR-able at the base** (carve out, land on main,
   rebase as they merge):
   - Log strategy: `LogStrategy (truncate|append|rotate)`, `ziti ops log-pipe`,
     append-across-restarts, rotate retention, start-hang fix.
   - Router connect bounded worker pool (`Fixes #3766`).
   - Link-manager perf: striped per-link locking + `RouterManager.Read`
     cache-before-txn.
   - Link subsystem -> slog.
   - Observability: `ctrl.is_leader` gauge, `pool.router.messaging` meters,
     slow-handler diagnostic, HA gauge-reads-0 fix.
2. **Link state over gossip** (`Fixes #3726`): core gossip replication + fault
   forwarding + listener-ordering race fix + canary versioning + inspect/
   `validate gossip` + superseded-iteration tombstones + reconcile + generic
   router gossip client + count-derivation + **#10 stability work** + **broadcast
   race fix** + chaos additions. (#10 = send timeouts, I/O-pool split for
   broadcast/digest off the apply pool, debounce digest re-trigger, pool rename;
   it's the stability that keeps gossip from collapsing under chaos — keep it in
   this PR, not separate.)
3. **Link metrics over gossip** (on `servermetrics`): lifecycle generalization +
   link-metrics store/publisher/listener + per-link metrics leak fix + routing
   cutover (`linkLatencyInGossip`).

## Integration status: DONE (2026-06-25) — merge resolved, builds + vets green

The `git merge --squash gossip-links` is fully resolved and staged on
`gossip-links-v5`. State:
- Main module `go build ./...` and `go vet ./...`: **EXIT 0**.
- zititest `go build ./models/links-test/` (single package; never recursive in
  zititest): **EXIT 0**.
- Targeted tests pass: `controller/gossip`, `controller/events`,
  `common/ctrlchan`, `common/servermetrics/metrics_pb`, `router/link`,
  `router/xlink_transport`.

How each piece landed (for reference):
1. **16 conflicted files resolved** (v5 side + folded gossip additions):
   ctrlchan, controller.go, controller/handler_ctrl/{bind,fault,router_link},
   network.go, router/ctrl_reporter, router/env/env, router/handler_ctrl/bind,
   router/link/link_registry, router/router, all 5 xlink_transport, validation.go.
2. **Channel-v5 handler-API port**: `binding.AddTypedReceiveHandler(x)` ->
   `channel.AddReceiveHandlers(binding, x)`; non-generic `channel.TypedReceiveHandler`
   -> `channel.ContentTypeReceiver`. Also fixed in auto-merged files
   (handler_peer_ctrl/bind, handler_mgmt/bind, both handler_diagnostic.go). The two
   slow-handler diagnostic wrappers dropped their now-redundant
   `AddTypedReceiveHandler`/`timedTypedReceiveHandler` (v5 routes typed handlers
   through the already-wrapped `AddReceiveHandler`).
3. **ctrlchan/channel.go**: took v5 `MultiChannel` underlay-counting API; dropped
   the v4-only `DialFailed`/`CreateGroupedUnderlay`/`HandleUnderlayAccepted`. The
   v4 `DialFailed` 50% reconnect jitter is now an opt-in `BackoffConfig.Jitter` in
   channel (openziti/channel#274, in PR #272). **Follow-up**: wire ziti's
   `NewDialCtrlChannel` to set `Jitter` once a channel/v5 release with it is tagged
   (ziti pulls v5.0.15 from the module cache).
4. **network.go**: kept `servermetrics.*` service counters + added the gossip
   fields (`GossipStore`, `LinkGossipType`, `LinkMetricsType`, `CanaryGossipType`,
   `gossipTypes`, `canaryListener`, `isHA`, `leaderCheck`).
5. **Phase 5 on servermetrics**: `router/ctrl_reporter.go` (now package `router`)
   uses `servermetrics`; added `bool linkLatencyInGossip = 13` to
   `common/servermetrics/metrics_pb/metrics.proto` and regenerated via `go generate`
   (not hand-edited). `AcceptMetricsMsg` honors it. Wire-compat test still passes.
6. **host_stats consolidation**: the gossip branch's `common/metrics` (package
   `metrics`: host_stats.go + linux/other) was folded into `common/servermetrics`
   (both controller and router import it; `pool_metrics.go` already lived there).
   This removed the third metrics package and the `metrics`-name alias collision;
   the gossip `fabricMetrics` alias is gone (all call sites use `servermetrics.`).

### Carve: DONE (2026-06-25)

The squash was carved into a stacked series on `gossip-links-v5` (base
`c9e58f232`, the servermetrics PR #4036). Each commit builds independently
(`go build ./...`; zititest single-package builds); the stack tree equals the
`gossip-links-v5-squash-checkpoint` backup branch (modulo this doc):

1. `3b7594777` Add striped per-link locking and cache-before-txn router reads (infra)
2. `0cda84407` Add ziti ops log-pipe command (infra)
3. `9ffa6f003` Add fablab log strategy and host-reachability-aware chaos helpers (infra)
4. `84b2026a4` Add link state replication over gossip. Fixes #3726
5. `cb68a9c12` Add link metrics over gossip

Pragmatic-split notes (targeted carveouts possible later):
- slog migration, observability (is_leader gauge / pool meters / slow-handler
  diagnostic), bounded router-connect pool, and the #10 stability + broadcast
  fix all ride in commit 4 (entangled with gossip in network.go/controller.go/
  router.go; not separable into independent base PRs without heavy hunk-surgery).
- The `ctrl_pb.LinkMetrics` proto message lives in commit 4's regenerated
  `ctrl.pb.go` (the regen is dominated by link-state changes; protoc version drift
  makes a per-stage regen produce huge noise). The link-metrics *logic* and the
  servermetrics `linkLatencyInGossip` field are cleanly in commit 5.

Backup branch `gossip-links-v5-squash-checkpoint` retains the single squash commit.

## Link-metrics-over-gossip implementation status

(Sub-phases done on gossip-links; carry through the rebase.)
- Lifecycle generalization (StateTypeInfo widened; per-store-type owner-drop /
  epoch-cleanup / connect-digest): **done** (`0c669bac9`).
- Link-metrics store (`LinkMetrics` proto in ctrl_pb) + controller listener
  (exact-iteration apply, both-end reconcile) + router publisher (threshold +
  force-on-iteration) + inspect key: **done** (`d875016c8`).
- Phase 5 (router sets `linkLatencyInGossip`; `AcceptMetricsMsg` skips latency
  extraction when set): **WIP** (`d5e637a2f`) — must be reworked onto
  `servermetrics` during the integration (item 5 above).
- Design rollout phase 2 (narrow metrics firehose to the subscription controller)
  and phase 3 (remove the latency block from `AcceptMetricsMsg`): **future**.

## Key decisions

- `servermetrics`: package name == dir, plain imports (no aliases); base metrics
  imported plain as `metrics`. Distinct proto package (`ziti.common.servermetrics.pb`),
  wire-compatible with the library proto.
- Keep #10 stability with the link-state-gossip PR (not separable).
- gossip is unreleased -> the broadcast fix needs no main backport.
- The fablab `links-test` instance (3 ctrls + 400 routers) may still be up; ctrl1
  crashed, ctrl2/ctrl3 were alive and converged.
