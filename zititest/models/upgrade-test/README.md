# upgrade-test model

## Goal

Stand up a full smoke-style OpenZiti topology on a parameterized "from" version using legacy
authentication, confirm traffic is flowing, then walk the system through a sequence of in-place
upgrades while asserting that traffic keeps flowing throughout:

1. Start on the "from" version (default: latest 1.6.x) with a single standalone controller, legacy auth.
2. Upgrade the controller to the "to" version (default: v2.0.3). This auto-enables OIDC.
3. Upgrade the routers to the "to" version.
4. Convert the controller to HA (single-node cluster).
5. Add two more controller nodes to form a 3-node cluster.
6. Optionally upgrade the whole cluster again, to the "next" version (default: 2.1 built from `main`).

The ZET (ziti-edge-tunnel), ERT (edge-router-tunneler), and ziti tunnel clients and hosts stay up
across the entire sequence. The whole run is repeatable in a loop so we can flush out timing errors
and race conditions.

## Topology

All AWS hosts are provisioned up front. Adding AWS instances mid-run is avoided; the two extra
controllers simply sit idle until the "add nodes" phase.

Controllers:
- `ctrl1` - runs from the start.
- `ctrl2`, `ctrl3` - provisioned but not started until the add-nodes phase.

Routers (same as smoke):
- `router-east-1` - ERT client.
- `router-east-2` - initiator.
- `router-west` - ERT host.

Clients and hosts that must stay up across the whole sequence, each present as a **pair** so that
one instance can be restarted while the other keeps serving (see OIDC verification). Each pair is named
`<name>-stable` and `<name>-restart`, and carries a `stable` or `restart` tag so the restart and
PID-persistence actions can select the whole set by role:
- `loop4-client-stable` / `-restart` - Go SDK sim dialer.
- `loop4-sdk-host-stable` / `-restart` - Go SDK sim listener (binds the `loop-sdk` service).
- `ziti-edge-tunnel-client-stable` / `-restart` and `ziti-edge-tunnel-host-stable` / `-restart` - ZET, the C SDK tunneler.
- `ziti-tunnel-client-stable` / `-restart` and `ziti-tunnel-host-stable` / `-restart` - Go SDK tunneler.

The two instances of each pair co-locate on the same host as each other, so pairing adds components
but no AWS instances. Process filters keep co-located instances individually controllable: ZET by its
config id, ziti-tunnel by `--cli-agent-alias`, and loop4 once its process filter includes the config id
(a small, backward-compatible change to `Loop4SimType`). The paired hosts binding the same service
simply create two terminators, so restarting one leaves the other serving.

Support components (single, not paired): plain-TCP loop4 backends that the tunneler hosts forward their
loop service to (see Liveness). `loop4-ert-backend` sits on the ERT host; the ZET and ziti-tunnel hosts
share a box and a single `loop4-tunnel-backend`. These have no ziti identity, so they are not part of the
OIDC story.

## Version parameterization

Both ends are fully parameterized (version X -> version Y), defaulting to latest 1.6.x -> v2.0.3.

- `fromVersion` / `toVersion` model variables drive the controller and router binaries.
- `zetVersion` for ZET, covering both its client and host instances. ZET is not upgraded mid-run; it
  stays up the whole time. Parameterizing it lets us sweep ZET versions across separate runs. Default is
  the version shipped with the current stable Windows desktop edge client: `v1.18.0` (bundled in Ziti
  Desktop Edge for Windows release 2.11.2.5, the latest stable release; note 2.11.2.7 bundles v1.18.2
  but is a pre-release).
- `nextVersion` is an optional second hop: once the 3-node cluster is up on `toVersion`, the whole
  cluster is upgraded again. `nextVersionSource` / `nextVersionRef` work exactly like their `toVersion`
  counterparts. Setting `nextVersion` to the empty string skips the phase and its two gates entirely.
- The existing per-component `<componentId>_version` label knob still works for one-off overrides.

Both the "from" and "to" binaries are pre-staged on every host during distribution. Because binaries
are versioned and coexist on-host (`ziti-<version>`, `ziti-edge-tunnel-<version>`), every upgrade is
just `SetVersion(newVersion)` followed by a restart, and a downgrade (for reset) is the same in reverse.
No mid-run binary rsync is needed. `ControllerType.Start` resolves the binary from the component's
current `Version` at start time, so a version swap plus restart is sufficient.

The Go SDK clients (`ziti-tunnel-*`, `loop4-*`) start on `fromVersion` and stay there. That is
deliberately the interesting compatibility case for the OIDC question below.

## Config strategy

One controller config works for both binaries. It has no explicit OIDC section, so the 2.0 binary
auto-binds the `edge-oidc` API on its first restart (`ensureOidcOnClientApiServer` in
`controller/controller.go`). That auto-bind is the "upgrade forces OIDC to become available" event, and
it happens with no config change.

For the HA conversion, the controller config is re-rendered to add the `cluster:` directive and rsynced
to the host before restart. The standalone binary then auto-migrates the bbolt DB into raft and comes up
as a single-node HA cluster.

Router controller-endpoint handling after the HA conversion (whether routers need all three controller
addresses in config or auto-learn them from the cluster) is verified during implementation.

## Liveness

We adopt the `circuit-test` harness: `SimServices` plus remote-controlled loop4 clients with
`iterations: -1` (run forever, auto-reconnect after restarts), streaming success/failure metrics every
five seconds.

To drive traffic through the ZET and ziti-tunnel hosting paths (not just the pure-SDK path), we define
services whose terminators are the ZET host and the ziti-tunnel host, each backed by a plain-TCP loop4
listener behind the tunnel:

```
loop4 dialer (SDK) -> ziti service -> ZET / ziti-tunnel host -> TCP -> loop4 listener
```

The circuit-test model already uses loop4 this way, so this is a proven pattern. We also keep a
pure-SDK-hosted service and an ERT-hosted service, so the sim continuously exercises all four hosting
flavors.

Quiescent-vs-churn validation (no failures when idle, tolerated during restarts). Rather than correlate
raw 5-second metric samples with churn windows, validation is expressed in terms of the sim's discrete
scenario runs (the remote-controlled pattern), which is far less flake-prone. After each disruptive step:

- Recovery gate: require N consecutive clean scenario runs (zero errors) achieved within a recovery
  timeout (default ~1 minute). This tolerates the transient failures expected while a controller or
  router restarts.
- Stability gate: once recovered, require scenario runs to continue clean for a stability window
  (default ~2 minutes) before advancing to the next phase.
- N, the recovery timeout, and the stability window are all tunable knobs.

The baseline (phase 1) and the final validation (phase 6) use the same clean-runs gate with no preceding
disruption.

## Phase sequence

The main orchestration action runs these phases in order.

0. Bootstrap on `fromVersion`: standalone `ctrl1`, routers, all clients and hosts enrolled; start; wait
   ready.
1. Baseline: start the continuous sim; assert clean steady-state. Traffic is flowing on legacy auth.
2. Upgrade controller to `toVersion`: stop `ctrl1`, `SetVersion`, restart (same standalone config, so
   OIDC auto-binds). Assert recovery and clean steady-state, then run the restart-one-of-each OIDC
   verification (see below).
3. Rolling router upgrade to `toVersion`: one router at a time (stop, `SetVersion`, start, wait) so
   traffic keeps flowing through the others. Assert recovery after each, clean steady-state after all.
4. Convert controller to HA (single node): stop `ctrl1`, swap in the cluster-directive config, restart
   so it auto-migrates to raft. Assert recovery and clean steady-state, then re-run the restart-one-of-each
   verification.
5. Add two nodes: start `ctrl2` and `ctrl3`, run `ziti agent cluster add` for each, wait for a stable
   three-voter quorum, update router (and possibly SDK client) controller lists if needed. Assert
   recovery and clean steady-state, then re-run the restart-one-of-each verification.
6. Final validation: full clean steady-state.
7. Optional cluster upgrade to `nextVersion`: upgrade all three controllers, assert clean steady-state,
   roll the routers, assert clean steady-state again. Skipped when `nextVersion` is empty.

   Not implemented: asserting that the *un-restarted* instance of each pair kept its original PID across
   phases 1-5, which would prove the long-lived clients truly stayed up rather than having been quietly
   restarted. The steady-state gate checks they are healthy, not that they are the same processes.

## OIDC verification via client restart

We verify OIDC by observing real clients rather than a synthetic probe. This is the only way to cover
ZET (the C SDK), and it exercises the exact auth path production clients use.

The mechanism relies on two facts:

- A long-lived Go SDK context caches the controller's advertised capabilities via `sync.Once` and never
  re-reads them on reconnect (`edge-apis/client_edge_client.go`). So a client that stays up across the
  1.6 -> 2.0 upgrade keeps using its legacy session; only a freshly started client re-reads capabilities
  and can switch to OIDC.
- The controller emits an `apiSession` event (`controller/event/api_session.go`) whose `type` field is
  `legacy` or `jwt` (jwt is the token/OIDC session), keyed by `identity_id` and `ip_address`. We enable
  this event with a file handler in `ctrl.yml.tmpl`, exactly like the entityChange/circuit/link/router
  events already there. (The `authentication` event's `type` is the credential kind, `cert`/`updb`/`ext-jwt`,
  not the session mechanism, so it does not distinguish legacy from OIDC; we use `apiSession`.)

Because every client and host is a pair, after the controller upgrade we restart the `-restart` instance
of each pair, leaving the `-stable` instance up:

- The `-stable` instance proves the client survives the upgrade on its existing legacy session, with no
  re-auth and no traffic gap (its PID is checked for continuity in the final phase).
- The `-restart` instance is a fresh process, so it re-authenticates. We read the new `apiSession`
  `created` event for that instance (matched by identity or source IP) and check its `type`.

Expected results: freshly started Go SDK clients (loop4, ziti-tunnel) default to dynamic OIDC detection,
so they should come back as `jwt`. ZET (C SDK, `v1.18.0`) auto-switch behavior is genuinely uncertain; if
a restarted ZET comes back `legacy`, that is a real finding, and the `apiSession` event makes it visible
either way. So ZET's expected value is TBD by design, the test documents actual behavior.

This replaces the earlier synthetic edge_apis probe and the standalone capability assertion: a restarted
client returning `jwt` already implies the controller advertises OIDC.

Phases 4-5 (HA and multi-controller) are the predicted breaking point for the long-lived legacy siblings.
"They keep working" is the pass criterion. If a later phase breaks them, that triggers an in-scope
sdk-golang fix to make long-lived contexts re-detect OIDC / handle the cluster on reconnect. The test is
designed to surface exactly that.

## Cluster upgrade and controller memory

The optional second hop upgrades an established 3-node cluster rather than building one, which is the
scenario production operators actually run and the one [#4219](https://github.com/openziti/ziti/issues/4219)
reports a controller OOM in. `clusterUpgradeMode` picks how it happens:

- `rolling` (default) restarts one node at a time, followers first and the leader last. Peers hold the
  cluster in mixed-version read-only mode until the last node is done, which is the state #4219 blames.
- `all-at-once` restarts every node together, so no node ever sees a version mismatch. This is the
  workaround the issue asks about, and it is also the control case: every node still cold-starts, so if
  memory climbs here too, the mismatch is not the mechanism.

No steady-state gate runs between nodes in rolling mode. The cluster is read-only while versions differ,
so anything needing a write would fail for reasons the phase is not testing. The gate runs once the whole
cluster is on `nextVersion`.

Every controller host samples its controller's RSS once a second for the whole iteration, writing
`~/logs/<ctrl>-mem.csv`. The controller is located by its agent alias, the same discriminator fablab's
process filter uses, so sampling follows the process across an upgrade and records a zero while it is
down: a node in a crash loop shows up as a sawtooth rather than a gap. Peaks are reported per node after
the cluster upgrade and again at the end of the iteration.

`ctrlMemory.heapDumpAtMb` captures a heap profile (`~/logs/<ctrl>-mem-<epoch>.pprof`) the first time RSS
crosses it. That profile is the thing #4219 is missing and the one artifact that cannot be recovered after
the fact. The samples and profiles are under `~/logs`, so CI collects them with the rest of the logs.

Two checks can fail the cluster upgrade, both disabled by setting them to zero:

- `ctrlMemory.failAtRatio` (default 3) compares each controller's peak over the upgrade against its own
  peak over the two minutes of settled traffic immediately before it. This is the one that bites at this
  scale, and it is the shape #4219 reports: roughly 5x a 150-190 MiB baseline. The default is a first
  guess, since a cold start legitimately overshoots a warm steady state; calibrate it against a run known
  to be good.
- `ctrlMemory.failAtMb` (default 1024) is the absolute ceiling, sized for the reported failure. It is a
  backstop, and is unlikely to be reached without a much larger data set.

`fablab exec ctrlMemorySummary` prints one row per controller per iteration for the whole run, plus the
highest peak seen and where, so a finished run can be read back with one command rather than scrolled for.
`fablab exec reportCtrlMemory` is the narrower form, covering only the current iteration.

Nothing is discarded between iterations. Samples accumulate in one file behind a `# start,<epoch>` marker
that scopes each report to the current iteration, and the profile path carries the sampler's start time,
so iteration 4 cannot delete the profile iteration 3 captured. At a line a second the samples cost roughly
1.7 MB per controller per day.

Scale caveat: this model runs a handful of routers, identities and services, so cold-start cost here is a
fraction of a production controller's. Expect the *shape* to be informative (does the upgraded node
diverge from its warm peers, and does it diverge in `all-at-once` too) rather than the magnitude. Chasing
the reported 1 GiB number would mean seeding a much larger data set first.

## Iteration and reset

A `testIteration` action runs `{reset -> full upgrade sequence}` once, and `fablab exec-loop` repeats it
for as long as asked. Rather than hand-rolling a surgical state wipe, reset leans on the existing fablab
machinery: wipe everything (controller DB and raft state, PKI/enrollment, cached tokens) and re-run
distribution and bootstrap from the `fromVersion` standalone configuration. This is more trustworthy
than trying to selectively clear state, and an incomplete wipe would produce cross-iteration
contamination that looks exactly like a race.

Each iteration is fully independent, which is what flushes timing and race issues. SDK clients restart
per iteration because the wipe invalidates identities, but they stay up across all phases within an
iteration, which is what matters for catching mid-upgrade problems. A full downgrade-in-place without a
wipe is not possible, since the 2.0 DB schema cannot be read by the 1.6 binary.

A bare `fablab exec testIteration` runs a single bounded pass; `fablab exec-loop <until> testIteration`
is the opt-in form for long local soak runs and a time-boxed slot in the validation suite.

## Longer-term direction

The version pair is a parameter, but the model is currently shaped around one specific transition:
1.6.x -> 2.0.x. The goal is to run LTS -> current instead, so each release is exercised against the
version most deployments are actually upgrading from, and the pair moves as LTS moves.

Two things stand in the way, both of which encode "this is the 1.6 -> 2.0 upgrade" rather than "this is
an upgrade":

- The transition-specific workarounds. `restartZetWorkaround` restarts ziti-edge-tunnel because versions
  at or below `zetRestartWorkaroundMaxVersion` cannot rebuild their edge sessions after the 2.0 JWT
  session migration, and `reconcileStaleTerminators` runs only while `anyPreV2Router` holds, cleaning up
  terminators that pre-2.0 routers drop without telling the controller. Both are gated on version so
  they switch themselves off, but a different pair needs its own set, and there is no structure yet for
  saying which workarounds belong to which transition.
- The phase sequence itself. The standalone -> HA conversion and the cluster-node join are steps in the
  1.6 -> 2.0 story specifically. An LTS -> current run where both ends are already HA would want a
  different sequence, so the phases need to become selectable rather than a fixed list.

Neither is large on its own, but together they are why the version defaults are not simply pointed at a
moving LTS target today.
