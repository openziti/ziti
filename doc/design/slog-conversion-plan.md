# Slog conversion plan

Companion to [logging-refactor.md](logging-refactor.md) and
[logging-refactor-progress.md](logging-refactor-progress.md). Where
the progress doc gives a coarse chunking order, this plan is the
code-grounded version: which packages convert in which order, what
channel names they take, and how the special cases (sdk-golang
embedders, cross-repo deps) get handled.

## Goals

1. Operators get per-channel debug control over every subsystem
   that has a stated triage value.
2. The conversion produces small, reviewable PRs. One chunk = one
   PR is the target.
3. Cross-repo deps (channel, transport, sdk-golang) convert in an
   order that lets ziti rebase cleanly when each lands.
4. SDK embedders can inject their own logger without
   `logging.Install`-ing global state.

Working assumptions (callable out if they need revising):

- **Default channel granularity is one channel per Go package.**
  Sub-channels appear only when there's a specific operational
  reason (high volume, separable failure modes inside the package).
- **The first three to four chunks get deep analysis here.** Later
  chunks get a sketch plus a "revisit after the first pass" note,
  since the early conversions teach us things we can apply later.

## Scope inventory

### ziti repo

Counts are `pfxlog.` call sites, approximate, gathered with
`grep -rh 'pfxlog\.' --include='*.go'`. The "channel" column is
the proposed name; multiple lines under one package mean splitting
into sub-channels.

| Package | Files | Sites | Proposed channel(s) |
|---|---:|---:|---|
| `controller/model` | ~30 | ~85 | `controller.model` (likely splits later by area: `.identity`, `.policy`, `.posture`) |
| `controller/internal/routes` | 22 | ~60 | `controller.edge.api` |
| `controller/handler_ctrl` | 24 | ~55 | `controller.ctrl` |
| `controller/handler_edge_ctrl` | 21 | ~50 | `controller.edge.ctrl` |
| `controller/db` | 15 | ~30 | `controller.db` |
| `controller/handler_mgmt` | 12 | ~25 | `controller.mgmt` |
| `controller/storage` | 11 | ~25 | `controller.storage` |
| `controller/network` | 9 | ~20 | `controller.network` |
| `controller/raft` | 7 | ~15 | `controller.raft` |
| `controller/oidc_auth` | 5 | ~10 | `controller.oidc` |
| `controller/handler_peer_ctrl` | 6 | ~10 | `controller.peer` |
| `controller/events` | 4 | ~10 | `controller.events` |
| `controller/sync_strats` | 3 | ~5 | `controller.sync` |
| `controller/env` | 4 | ~10 | `controller.env` |
| `router/xgress_edge` | 8 | 95 | `router.xgress.edge` |
| `router/state` | 14 | 58 | `router.state` |
| `router/env` | 3 | 35 | `router.env` |
| `router/xgress_edge_tunnel` | 5 | 25 | `router.xgress.edge_tunnel` |
| `router/handler_ctrl` | 11 | 21 | `router.ctrl` |
| `router/link` | 3 | 20 | `router.link` |
| `router/xlink_transport` | 5 | 13 | `router.link.transport` |
| `router/forwarder` | 3 | 11 | `router.forwarder` |
| `router/handler_link` | 6 | 9 | `router.link.handler` |
| `router/xgress_transport_udp` | 2 | 8 | `router.xgress.transport_udp` |
| `router/posture` | 3 | 7 | `router.posture` |
| `router/xgress_geneve` | 2 | 6 | `router.xgress.geneve` |
| `router/xgress_sdk` | 1 | 4 | `router.xgress.sdk` |
| Other `router/xgress_*` | — | ≤3 each | `router.xgress.*` (fold into one chunk) |
| `common/pb` | — | 22 | (generated code; do last, low value) |
| `common/agent` | — | 16 | `common.agent` (already touched by Phase 2) |
| `common/agentlog` | — | 8 | already migrated |
| `common/profiler` | — | 7 | `common.profiler` |
| `common/alert` | — | 5 | `common.alert` |
| `tunnel/*` | — | ~17 | `tunnel.*` (per package) |
| `ziti/cmd` | — | 74 | `cli.*` (per subcommand; low value, do as touched) |
| `ziti/run` | — | 10 | already migrated to `Install` |
| `ziti/enroll` | — | 6 | `cli.enroll` |
| `zititest/*` | — | ~60 | test code; convert only as test infra is touched |

**Total ziti-repo call sites: ~1500** (estimate; includes line
counts not unique-call-site counts).

### Other openziti repos

Counts here are from a `pfxlog org:openziti` search slice
(per_page=100, total=583). These are partial — the absolute counts
matter less than the relative ordering and which repos are
dependencies of ziti.

| Repo | Search hits (slice) | Position in dep graph | Notes |
|---|---:|---|---|
| `openziti/sdk-golang` | 28 | ziti depends on it | Embedder-injected logger required; see SDK pattern below |
| `openziti/channel` | 15 | ziti depends on it | Core message bus; converts cleanly with one channel per logical area |
| `openziti/transport` | 7 | ziti depends on it | TLS / TCP / quic; relatively small surface |
| `openziti/identity` | 3 | ziti depends on it | Identity, certs, configs; small |
| `openziti/xweb` | 4 | ziti depends on it | HTTP server framework |
| `openziti/metrics` | 2 | ziti depends on it | Metrics pipeline; small |
| `openziti/fablab` | 11 | test-only | Lab automation; convert as it's touched |
| `openziti/zrok` | 7 | independent product | Tracks ziti's conversion; not in scope here |
| `openziti/agora` | 2 | independent | Not in scope |
| `openziti/llm-gateway`, `openziti/mcp-gateway` | 2 each | new products | Not in scope |
| `openziti/ziti-doc` | 3 | docs/tutorials | Cosmetic; convert with code samples as they change |
| `openziti/dilithium`, `openziti/ziti-ops` | 1 each | rarely touched | Convert opportunistically |
| `openziti/agent` | 2 | archived (folded into ziti's common/agent) | No conversion needed |

For ziti's correctness, **only the four deps that ziti pulls into
its binaries matter**: `sdk-golang`, `channel`, `transport`,
`identity` (+ `xweb`, `metrics` to a lesser extent). Those are
the cross-repo critical path.

## SDK injection pattern

`sdk-golang` is consumed as a library by ziti embedders (zrok,
ziti-tunnel-sdk-c, third-party apps). Those embedders want control
over their own logging output: they don't want the SDK calling
`logrus.SetOutput(io.Discard)` or installing a global bridge under
their feet.

The SDK conversion follows a different pattern from the binaries.

### What the SDK exports

```go
// Package logging in sdk-golang. Mirrors the surface of
// common/logging but does NOT install anything globally.

package logging

// SetLogger installs the *slog.Logger the SDK uses for all of its
// own log lines. Default is slog.Default(), which inherits the
// embedder's existing setup. Calling this is optional; embedders
// who haven't migrated to slog can leave it alone.
func SetLogger(logger *slog.Logger)

// For returns a channel-scoped logger derived from the configured
// root. The "channel" attr is set to name. Operators of an
// embedding application can adjust per-channel levels via whatever
// surface that application exposes; the SDK does not provide one.
func For(name string) *slog.Logger
```

Inside the SDK, every package does `var log = logging.For("ziti.sdk.<area>")`
just like the ziti binary packages do, but the `logging` package
here is `sdk-golang/internal/logging`, not `ziti/v2/common/logging`.

### What the SDK does NOT do

- No `Install` function. SDK never touches the embedder's logrus
  state.
- No AsyncHandler by default. The SDK uses whatever handler the
  embedder's `*slog.Logger` already has. Embedders who want async
  buffering bring their own.
- No agent integration. The agent surface is application-level; an
  embedder that wants per-channel control over the SDK plumbs that
  via its own code.

### Migration step for sdk-golang

The conversion is a sequence:

1. Add `sdk-golang/internal/logging` with `SetLogger` + `For`.
   Default root is `slog.Default()`.
2. Add a logrus-to-slog bridge inside the SDK, **scoped to the SDK's
   own logrus calls only**. The SDK's `pfxlog.Logger()` calls flow
   through the SDK's bridge to the SDK's configured `*slog.Logger`.
   This is so the SDK can convert package-by-package without
   forcing embedders to migrate all at once.
3. Convert SDK packages to `logging.For(...)`. One package per PR.
4. Once all SDK packages are converted, remove the SDK-internal
   bridge.

The SDK's internal bridge differs from ziti's bridge in scope: it
hooks only the SDK's logrus instance (or none if the SDK isolates
itself to its own logger), not `logrus.StandardLogger`. Open
question for the first SDK PR: does the SDK already use a
non-standard logrus logger, or does it use `pfxlog.Logger()` which
goes through the standard one?

## Channel taxonomy

The naming convention is `subsystem.area[.sub-area]`, lowercase,
dot-separated. Proposed full set (grouped to make the operator's
view of available toggles legible):

**controller.\*** — controller subsystems:
- `controller.ctrl` — control-channel handlers
- `controller.edge.api` — edge REST endpoints (routes)
- `controller.edge.ctrl` — edge control-channel handlers
- `controller.mgmt` — management channel
- `controller.network` — network model
- `controller.model` — entity model (may split further on first pass)
- `controller.db` — boltdb / persistence
- `controller.storage` — controller storage layer
- `controller.raft` — raft / clustering
- `controller.oidc` — OIDC auth
- `controller.peer` — peer-controller links
- `controller.events` — event bus / dispatch
- `controller.sync` — sync strategies
- `controller.env` — controller env / boot

**router.\*** — router subsystems:
- `router.ctrl` — control-channel client side
- `router.link` — link state + dial/accept
- `router.link.transport` — xlink_transport implementation
- `router.link.handler` — control-channel link handlers
- `router.forwarder` — circuit forwarder
- `router.state` — router state mgmt
- `router.env` — router env / boot
- `router.xgress.edge` — edge-side xgress
- `router.xgress.edge_tunnel` — embedded tunneler xgress
- `router.xgress.transport_udp` — UDP xgress
- `router.xgress.geneve` — geneve xgress
- `router.xgress.sdk` — sdk-backed xgress
- `router.xgress.*` (others) — small variants
- `router.posture` — posture checks
- `router.metrics` — metrics
- `router.inspect` — inspect handlers

**fabric.\*** — fabric core (cross-repo for now; lives in sdk-golang/xgress):
- `fabric.xgress` — the xgress data path itself

**tunnel.\*** — embedded tunneler in `ziti tunnel` and in router's
embedded mode:
- `tunnel.intercept`, `tunnel.dns`, `tunnel.host`, etc.

**common.\*** — shared infrastructure:
- `common.agent` — agent listener + dispatch
- `common.profiler`, `common.alert`, `common.metrics`

**cli.\*** — ziti CLI subcommands:
- `cli.enroll`, `cli.edge.<verb>`, `cli.fabric.<verb>`, etc.
  Most of these are one-shot and don't benefit much from per-channel
  control, but the naming is consistent.

**ziti.sdk.\*** — SDK call sites (live in sdk-golang):
- `ziti.sdk.identity`, `ziti.sdk.controller`, `ziti.sdk.edge.api`,
  `ziti.sdk.xgress`, `ziti.sdk.tunnel`, etc.

**channel.\*** — channel/v4 internals (cross-repo):
- `channel.framing`, `channel.binding`, `channel.dispatcher`, etc.

**transport.\*** — transport/v2 internals (cross-repo):
- `transport.tls`, `transport.tcp`, `transport.quic`, etc.

## Conversion order

Roughly highest-value-first, where value = operational triage
benefit + bridge fast-path benefit + how often this code shows up
in chaos traces.

### Chunk 1: `router.link` + `router.link.transport` + `router.link.handler`

**Files**: `router/link/`, `router/xlink_transport/`,
`router/handler_link/` — ~14 files, ~42 sites.

**Why first**: The progress doc calls out `linkState.updateStatus`
as the proof-of-pattern conversion bootstrap. Link state, dial,
and accept paths are also where the gossip-links work happens, so
converting this first means gossip-links lands on slog directly.

**Channels**:
- `router.link` — overarching link state, link map management.
- `router.link.transport` — xlink_transport-specific (dial, accept,
  TLS handshake).
- `router.link.handler` — control-channel handlers that mutate link
  state.

**Hot-path watch**: link state churn under chaos. Any new
Warn/Error in this conversion gets bounced; resilience messaging
("link X dropped, retrying") at Info is fine because it's rate-
limited by the underlying event.

**Tests**: the existing link tests (`tests/link_test.go`,
`testutil/linkschecker.go`) stay on logrus assertions; we update
them to slog assertions in the same PR.

### Chunk 2: `router.xgress.*`

**Files**: `router/xgress_edge/` (95 sites — heaviest),
`router/xgress_edge_tunnel/`, `router/xgress_transport_udp/`,
`router/xgress_geneve/`, `router/xgress_sdk/`, smaller variants.

**Why second**: per-payload paths in xgress. The progress doc
notes "Fabric Debug volume often originates here". Converting lets
operators surgically enable Debug for one xgress flow without
flooding everything.

**Channels**: one per package, prefixed `router.xgress.`. The
implementations are independent enough that single-channel-for-all
loses the operational benefit.

**Hot-path watch**: this is the highest-volume conversion in the
plan. Carefully audit each new log line; nothing new at Warn/Error
per packet. Existing Debug lines stay Debug.

**Note**: core xgress data path lives in `openziti/sdk-golang/xgress`,
not in ziti. That part converts under Chunk 6 (SDK).

### Chunk 3: `controller.ctrl` + `controller.edge.ctrl` + `controller.peer`

**Files**: `controller/handler_ctrl/` (~55 sites),
`controller/handler_edge_ctrl/` (~50 sites),
`controller/handler_peer_ctrl/` (~10 sites).

**Why third**: Control channels are visible in chaos traces and
have a clean subsystem boundary. Three packages, three channels;
all three convert together because they share helpers.

**Channels**:
- `controller.ctrl` — fabric-side control handlers
- `controller.edge.ctrl` — edge control handlers
- `controller.peer` — peer-controller (HA / clustering)

**Tests**: handler test files in each package convert in the same
PR.

### Chunk 4: `controller.network` + `router.forwarder` + `controller.events`

**Files**: `controller/network/`, `router/forwarder/`,
`controller/events/`.

**Why fourth**: the digest / forwarding / event paths the design
doc highlighted from chaos traces. Smaller per-package than the
edge handlers but operationally important.

**Channels**: `controller.network`, `router.forwarder`,
`controller.events`.

### Chunks 5+: sketch only

The rest, in rough order. Each gets its own PR; the channel name
is the row in the inventory above.

5. **`controller.raft` + `controller.storage` + `controller.db`** —
   persistence layer. Convert together because they're tightly
   coupled and the operational use case is "show me what raft is
   doing" which spans all three.
6. **sdk-golang** — see SDK section. Larger surface; do as its own
   stream of PRs in the SDK repo.
7. **`controller.model` + `controller.internal.routes` +
   `controller.handler_mgmt`** — edge model + REST. Largest by
   call-site count, but mostly Info-level boilerplate. Less hot-path
   than the others; do once the bridge has been measured.
8. **`router.state` + `router.env` + `controller.env`** — boot /
   state mgmt; mostly Info-level startup messages.
9. **transport/v2** — cross-repo. TLS handshake-EOF site lives here;
   conversion lets operators enable Debug for the TLS subsystem
   specifically during DoS triage.
10. **channel/v4** — cross-repo. Smaller surface but foundational.
11. **`common.*` packages** — agent, profiler, alert, etc.
    Convert each as its own small PR.
12. **`tunnel.*`** — embedded tunneler.
13. **`ziti/cmd`** — CLI subcommands. Per-channel control is
    low-value here (one-shot commands), but the naming consistency
    is worth doing as commands get touched for other reasons.
14. **Generated code (`common/pb`)** — convert last; mechanical and
    low-value.
15. **`zititest/*`** — test infrastructure. Convert as test infra
    gets touched for other reasons.

## Cross-repo coordination

The dep order matters because each downstream rebases on its
upstreams. Two paths:

**Path A — bridge holds, ziti converts first.** ziti's
`common/logging` foundation is already merged. ziti can convert
its own packages (chunks 1–5, 7, 8) without touching the deps,
because un-migrated dep call sites flow through the bridge at the
global level. Then sdk-golang, transport, channel convert at their
own pace and ziti rebases.

This is the default path and the one the conversion order above
assumes.

**Path B — bridge holds, deps convert first.** Possible if a
specific dep change is needed earlier (e.g. transport.tls debug
to triage a production issue). Each dep PR stands alone and ziti
picks them up on its next dep bump.

The plan does not commit to a specific ordering between the two
paths; they can interleave.

## Per-conversion PR checklist

Reproduces the checklist in [logging.md](../logging.md) so it
shows up in this plan too:

- [ ] All `pfxlog.Logger()` / `pfxlog.ContextLogger(...)` calls in
      the package replaced by a package-scoped
      `var log = logging.For("subsystem.area")` or by a context-
      derived child (`log.With("circuit", c.Id)`).
- [ ] Channel name documented at the top of the package (godoc
      comment).
- [ ] No level changes. Lines stay at the level they had.
- [ ] No new Warn/Error at per-event hot-path rate.
- [ ] Tests updated to use slog assertions (`slog.Record` /
      `slog.Logger.Enabled` / a recording handler) instead of
      logrus-based ones.
- [ ] Where the package's bridged behavior was operator-visible
      (per-channel pfxlog overrides), the conversion PR mentions
      that operators wanting that surface back must use the agent's
      slog channel name.

## Open questions to resolve as we go

- **Sub-channel granularity inside `controller.model`.** ~85 sites
  in one package is too coarse to be useful. First PR proposes a
  split (likely `.identity`, `.policy`, `.posture`, `.config`,
  `.service`), reviewed against actual sites.
- **SDK logrus instance.** Does sdk-golang already use a non-
  standard logrus logger, or does it call `pfxlog.Logger()` which
  goes through `logrus.StandardLogger`? Answer affects how the
  SDK's bridge isolation works. Investigate at the start of the
  SDK conversion.
- **fablab call sites.** The 11 hits in `openziti/fablab` are
  test-infrastructure-only; do we convert them as a single PR or
  per-test-area? Decide after the ziti chunks settle.
- **Whether to file a tracking issue per chunk.** The progress doc
  raised this question without answering it. Default: yes, one
  tracking issue per chunk, opened when the chunk is next in the
  queue. Avoids 15+ open issues sitting cold.

## Out of scope (deferred per the foundation design)

- PC-based method/file level overrides
- OTel adapter
- Persistent yaml-driven level overrides
- pfxlog removal (the package stays on the dep graph; the bridge
  handles its remaining calls indefinitely)
