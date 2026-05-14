# Controller-Managed Router Config: Outstanding Work

The design lives in `doc/design/ctrl-managed-router-config.md`. This file tracks deferred items
and TODOs — completed phases are gone (`git log` is the record of what landed).

## Changes already landed

- **`ConfigType.Target`** field distinguishes router-targeted from service-targeted config types;
  immutable after creation. Built-in types and the migration default to `service`.
- **`Router.Configs []string`** on the base router model, with one-config-per-type enforcement and
  the stricter requirement that referenced configs resolve to a config type with `Target =
  "router"`. Config deletion walks referencing routers and rewrites their `Configs` field.
- **`router.link.v1` built-in config type** with full JSON schema for listeners, dialers,
  heartbeats, queue sizes, and `gcMode`.
- **`DataState.Router`** message added to the RDM protobuf; routers are first-class entities in
  the RDM event stream.
- **Per-router `Config` filtering at `RouterSender`** so each router only sees its own configs.
  Everything else (Router entities, identities, services, policies) keeps broadcasting.
- **Router-side `Routers` map** populated from RDM events; available to router subsystems.
- **`managedConfig.allow`** allow-list on the local router YAML, with empty/`all`/explicit-list
  semantics. Empty means controller-managed config is disabled entirely.
- **`router/managedconfig` registry** with source-tracked apply (`SourceLocal` / `SourceController`),
  base-level local-wins precedence, version selection, rollback on apply failure, and an inspect
  surface (`ziti fabric inspect <router> managed-config`).
- **`router.link.v1` handler** (`router/link/FactoryRegistry`) implements `ConfigHandler`. Apply
  rebuilds the listener / dialer set wholesale; established Xlinks survive because
  `Listener.Close()` only closes the accept loop (Phase 4a). Local YAML is translated to JSON
  and pushed through the registry as `ApplyLocal` at startup.
- **`UpdateLinkListeners`** ctrl message (`1056`) propagates the router's listener set to the
  controller on every Apply that changes listeners; controller re-fans via the existing
  `PeerStateChange` path so other routers see the update. Dialer changes trigger
  `RescanForDialOpportunities` locally.
- **Structured `xlink.LinkKey`** (`{DialerBinding, Protocol, DestId, ListenerBinding}`) replaces
  the parsed string key on `Xlink`. Used by all staleness / GC checks.
- **`CheckStaleLinks` ctrl message + `ziti ops verify stale-links` CLI** for operator-driven,
  two-sided staleness verification with optional `--gc`.
- **Auto-GC via `router.link.v1` `gcMode`** (`preserve` / `orphaned` / `changed`). Router walks
  its xlinks after every Apply that mutates listeners, dialers, or gcMode, and closes
  one-sided-stale entries under the configured mode.
- **Fablab RDM test extended** to cover router configs, including chaos scenarios (controller
  restarts, RDM cache misses, router reassignment).
- **Legacy pre-0.30 controller compatibility** dropped from `link.Registry.GetLinkKey` and the
  duplicate-link fault path.

---

## Deferred: managed config delivery for non-edge routers

The RDM transport is in place; the gate is on the controller side. Managed configs only flow to
edge routers today. Operators of transit routers must still configure links via local YAML.

### Limitation

`controller/env/broker.go:RouterConnected` only invokes
`routerSyncStrategy.RouterConnected(edgeRouter, router)` when an EdgeRouter record is matched by
fingerprint. Transit routers fall into the `else` branch and are never added to the strategy's
`rtxMap`. Their RDM subscribe requests are rejected with *"received subscribe from router that is
currently not tracked by the strategy, dropping subscribe"*; no Config events flow.

Historically the sync strategy was for distributing edge data (identities, services, policies,
edge-target configs). Phases 2a–2c made the RDM the channel for general router config too, but the
broker gate hasn't been relaxed.

### Approach

**Option A — type-aware single strategy** (smaller change, recommended for first cut):

- Broker calls `strategy.RouterConnected(maybeEdgeRouter, router)` unconditionally;
  `maybeEdgeRouter` may be nil.
- `InstantStrategy.RouterConnected` handles nil edgeRouter: creates the rtx entry, drives RDM
  subscription, skips edge-only setup (identity tracking, tunneler flag, etc.).
- `filterEventsForRouter` drops edge-only event types (`Identity`, `ServicePolicy`, `Service`,
  `PostureCheck`, `Revocation`) when the destination router is non-edge.
- Edge-only event handlers (`ApiSessionAdded`, etc.) keep working — they already walk `rtxMap`
  selecting edge routers.

Net: one strategy, one RDM, one `rtxMap`. Transit routers join the tracker but get a stripped
feed.

**Option B — split concerns** (cleaner, do when complexity grows):

- Split `RouterSyncStrategy` into `RouterStateSync` (general router state + Router/Config events)
  and `EdgeDataSync` (identity/policy/posture/revocation).
- Two implementation files; `sync_instant.go` becomes a thin composer.
- Broker calls both as appropriate.

---

## Deferred: collapse Edge Router / Transit Router into Router

With controller-managed config, whether a router runs the edge subsystem is determined by whether
it has `router.xgress.edge`. Whether it tunnels is determined by `router.xgress.tunnel`. The router
type in the data model shouldn't dictate capabilities — but today it does, via the
`Router → EdgeRouter`/`TransitRouter` hierarchy and the `IsTunnelerEnabled` boolean.

### Current shape

- **Router** (base): name, fingerprint, cost, noTraversal, disabled, ctrlChanListeners, interfaces.
- **Edge Router**: adds roleAttributes, isVerified, certPem, isTunnelerEnabled, appData,
  unverifiedCertPem, unverifiedFingerprint. Enrolled via `erott`. Participates in EdgeRouterPolicy
  and ServiceEdgeRouterPolicy.
- **Transit Router**: adds isVerified, isBase, unverifiedFingerprint, unverifiedCertPem.
  Enrolled via `trott`. No role attributes, no policy participation.

After stripping the naming: Transit = Router + enrollment; Edge = Router + enrollment + role
attributes + policy participation + optional tunneler identity.

### Collapse

Fold Edge Router and Transit Router into the base Router; differences become config-driven. Single
enrollment method replaces `erott` / `trott`. API collapses to `/routers`. EdgeRouterPolicy /
ServiceEdgeRouterPolicy reference Router directly (or get replaced by unified router policies).

### The identity question

Today only tunneler-enabled edge routers get a matching Identity. With a unified type:

- **Option A — identity tied to tunnel config.** Identity and system policy are created/destroyed
  when `router.xgress.tunnel` is added/removed. Preserves current behavior; trigger moves from
  `IsTunnelerEnabled` to a config-change handler.
- **Option B — every router gets an identity.** Simpler. Routers always authenticate as
  themselves, appear uniformly in policies, can authenticate to external systems with their Ziti
  identity. Cost: every router creation also creates an identity, the system edge router policy
  always exists, existing transit routers need identities created during migration.

Option B is cleaner long-term. Decision deferred.

### Migration

Move Edge / Transit-specific fields into the base Router bucket. Drop the `edge` and
`transitRouter` child buckets. Consolidate enrollment records to a single method. Under Option B,
create identities + system policies for existing routers that don't already have them.
`/edge-routers` and `/transit-routers` become aliases of `/routers` for a transition period.

---

## Deferred: friendly CLI for router.link.v1 configs

Today the only path to creating a `router.link.v1` config is hand-typed JSON:

```bash
ziti edge create config my-link router.link.v1 '{"listeners":[{"binding":"transport","bind":"tls:0.0.0.0:6000","advertise":"tls:1.2.3.4:6000"}],"dialers":[{"binding":"transport","options":{"connectTimeout":"30s"}}]}'
```

Fiddly: nested JSON, multiple sections, nested objects for channelOptions and backoff, duration
strings, easy to typo a key. Schema validation only fires after submit.

### Proposed shape

1. **`ziti edge create router-link-config <name>`** with flags for the common single-listener
   single-dialer case. `--from-file <path>` (YAML or JSON) for complex configs. Listeners and
   dialers can each be specified multiple times via repeated `--listener=...` / `--dialer=...`.
2. **`ziti edge edit router-link-config <name>`** opens the config as YAML in `$EDITOR`. Round-trip
   via the `router/link/config.go` typed struct + a YAML marshaler. Schema-validate on save.
3. **No wizard** in the first cut — flags + edit cover the cases. Wizard can come later if
   operators ask.

### Out of scope

- Editing config-to-router assignment (`ziti edge update edge-router --configs` already exists).
- Schema discovery (`ziti edge list config-types --schema router.link.v1`). Useful but separate.
- Auto-generation from local YAML (operator hands the CLI a router's YAML, gets the equivalent
  `router.link.v1` config). Also useful, also separate.

---

## Smaller follow-ups

### Stale-link / auto-GC

- **Fablab integration test**: extend the smoke or chaos test with a "rotate listener address,
  verify previous link is reported stale, GC closes it without disturbing established traffic"
  scenario. The single-router `tests/` harness can't reproduce a real accepted link without
  injection hooks into both `Network.Link` and `xlink.Registry`.
- **`stale-links` event stream**: push each new staleness verdict as it happens instead of
  requiring a manual scan. Useful for observability dashboards.
- **Periodic auto-GC sweep**: today auto-GC only fires on `ConfigurationChange`. Drift discovered
  through peer-listener updates (e.g. peer renamed binding) won't trigger an auto-GC until our own
  config changes. If that surfaces as a real gap, add a periodic timer anchored on
  `UpdateLinkDest` events.
- **Per-listener / per-dialer gcMode override**: today there's a single top-level `gcMode`.
  Per-binding policies can be added if needed.
- **Controller alert on auto-GC closure**: not needed for v1 — log entries are sufficient. The
  alert hook from the managedconfig registry is available if we want to wire it later.

### Hot YAML reload safety

- **`RouterCapability.ControllerManagedConfig`** — declare in `ctrl.proto`, advertise via the
  Hello CapabilitiesHeader. Today's trigger paths are safe without it, but if/when hot-reload of
  local YAML lands, routers with managed-config enabled would publish `UpdateLinkListeners` even
  to old controllers. The capability lets controllers detect support and gates future
  controller→router managed-config messages.

### Changelog

- New `ctrl_pb.ContentType_UpdateLinkListenersType (1056)` — router → controller, fire-and-forget.
- Routers with `managedConfig.allow` set now republish listeners to the controller after every
  link subsystem change. Old controllers log an unknown-content-type warning but stay functional.
- Router-target Config reassignment via `PATCH /edge-routers/{id} configs=[...]` now drives a
  runtime listener rebuild on the affected router; previously required a router restart.
- Bug fix: GC of orphaned router-target configs in the router's RDM now correctly dispatches the
  remove event to the managed-config subscriber (was silently leaving listeners bound).
- `link.ConfigFromLocalYaml` no longer counts auto-filled heartbeat defaults as "local content";
  previously this suppressed controller management for routers without an explicit `link:` YAML
  section.

---

## Future work

Items in the design doc's `Open Questions` (templates / personas, circuit disruption on
disruptive changes, splitting config CRUD permissions along the `Target` axis) don't have an
active implementation plan yet.
