# Controller-Managed Router Configuration

## The Problem

Router configuration lives entirely in a local YAML file. To change behavior, you edit the file and
restart (or sometimes reload) the router. That doesn't scale, and there's no central place to define
"this is how routers in my network should behave." The result is configuration drift, manual toil,
and no story for fleet-wide changes.

The goal: manage router configuration from the controller. Everything except the controller endpoints
themselves (which already come from controller-pushed updates).

## What Needs to Be Figured Out

1. How do we split up the configuration?
2. How do we store it?
3. How do we propagate it to routers?
4. How do we apply it on the router side?

## Splitting Up the Configuration

A router's configuration has several distinct subsystems:

- **Link**: router-to-router link listeners and dialers
- **Xgress bindings**: traffic ingress/egress points (proxy, transport, edge, tunnel, etc.)
- **Edge**: SDK connection handling, session management
- **Forwarder**: data plane tuning
- **Metrics**: reporting intervals and queue sizes
- **Health checks**: ctrl ping, link checks
- **Transport**: TLS/DTLS configuration
- **Miscellaneous**: profiling, plugins, connect events, interface discovery

Not all of these belong under the same config type. Each major subsystem gets its own:

| Config Type               | What It Covers                                              |
|---------------------------|-------------------------------------------------------------|
| `router.link`             | Link listeners, dialers, heartbeats                         |
| `router.forwarder`        | Forwarding pool sizes, timeouts, rate limiting              |
| `router.metrics`          | Reporting intervals, queue sizes                            |
| `router.healthchecks`     | Ctrl ping check, link check intervals                       |
| `router.transport`        | TLS/DTLS transport-level settings                           |
| `router.xgress.<binding>` | Per-xgress-type configuration (one per binding type)        |

The `router.xgress.<binding>` pattern lets each xgress type (proxy, transport, transport_udp, edge,
tunnel, etc.) define its own config type and schema. The router dispatches by stripping the
`router.xgress.` prefix and looking up the binding in the xgress registry. Custom xgress
implementations follow the same convention; no extra metadata is needed.

The tunnel binding already works this way. Edge currently has its own top-level `Edge` config in the
router YAML; we consolidate that into `router.xgress.edge` so all xgress bindings follow the same
pattern.

### Config as a Feature Toggle

The presence or absence of a config drives whether a subsystem is enabled. A router with
`router.xgress.tunnel` runs the tunneler. Remove the config and the tunneler shuts down. There's
exactly one place to look to see whether a given binding is on or off.

## Storage

### Reusing the Config Type Infrastructure

The existing `ConfigType` system (used for service configs like intercept and host) is reused. The
piece that was missing: distinguishing "this config type is meant for a service" from "this config
type is meant for a router." We added a `Target` field to `ConfigType`:

```go
type ConfigType struct {
    boltz.BaseExtEntity
    Name   string                 `json:"name"`
    Schema map[string]interface{} `json:"schema"`
    Target string                 `json:"target"` // "service" (default), "router"
}
```

The `Target` field is immutable after creation. UIs filter by target; the API enforces that you
can't associate a tunneler intercept config with a router or a router link config with a service.

All built-in router config types have JSON Schema definitions; the controller rejects invalid
config before it reaches a router.

### Associating Configs with Routers

The base `Router` model gained a `Configs []string` field (matching `Service.Configs`). All router
flavors share the association mechanism via their embedded `Router`.

- One config per config type per router (same rule services follow).
- Each referenced config must resolve to an existing config type with `Target = "router"`.
  Stricter than services, which tolerate dangling config-type references.
- The validator short-circuits when `Configs` isn't part of the update field set, so unrelated
  router updates (e.g. control-channel listener updates from the router) don't pay the cost.
- Config deletion walks every referencing router, rewrites `Configs`, and re-`Update`s the router
  so entity-update listeners see the change. Mirrors the existing service-cleanup loop.

### Handling Arbitrary Xgress Types

Custom xgress types create their own config type with `target = "router"` and the
`router.xgress.<binding>` naming convention. The router strips the prefix and looks up the binding
in the xgress registry. If the factory exists, it's started with the provided config. If not, a
warning is logged.

### Versioning

Config type names include a version suffix: `router.link.v1`, `router.xgress.edge.v1`. The rules
let old routers and new configs coexist:

1. **Additive within a version.** New fields can be added; older routers ignore them.
2. **No removals or semantic changes within a version.** Fields can be deprecated but keep working.
3. **Breaking changes require a new version.** `router.link.v1` becomes `router.link.v2`.
4. **Routers select the highest version they understand.** A v3 router with both v1 and v2
   associated uses v2.
5. **Internally, the router uses a single config format.** It populates one internal struct from
   the highest-version config it understands — same pattern as host configs.

During a rolling upgrade, both versions can be associated. Old routers pick v1, upgraded routers
pick v2. Drop v1 once the fleet has caught up.

## Propagation

### Distributing Routers in the RDM

Routers are first-class entities in the RDM (`DataState.Router` message: id, name, fingerprint,
configs). This gets us two things:

1. **Fingerprint distribution** so routers can validate link peers against the controller's
   authoritative set.
2. **A delivery channel for router configs.** The same event/replay infrastructure that distributes
   services and identities now also distributes router configs.

### Per-Router Filtering

The motivation for filtering is **blast radius, not memory**. Back-of-envelope for 1000 routers /
~10k identities / ~1k services / ~5k configs / ~2k policies puts the RDM at 100-200 MB. Memory
isn't the bottleneck on modern hosts. We do want to avoid leaking per-router secrets (binding
addresses, credentials, tuning) to every other router.

`Config` events are filtered per-router by `RouterSender` at send time. Everything else (identities,
services, policies, `Router` entities) broadcasts as before. The shared `RouterDataModelSender`
stores everything; each `RouterSender` wraps a router's connection and filters as it sends.

### Change Notification Flow

Two distinct triggers cause a `Config` event to flow:

1. **A router's `Configs` list changes.**
   - Emit the `Router` event with the new IDs.
   - For newly-added IDs, also emit the full `Config` entity to that router.
   - For newly-removed IDs, nothing extra on the wire. The receiver GCs router-target Configs
     synchronously when it sees its own shrunk `Router` event. Service-target configs are left
     alone. This keeps the receiver aligned with its assignment without sending synthetic remove
     events.

2. **A config's data changes.** Emit the `Config` event to every router currently referencing it.

Router entities themselves (id, name, fingerprint, cost) broadcast to all routers so every router
can validate link peers.

On reconnect, the router sends its current RDM index. If the controller's event cache can replay
forward from there, it streams the deltas (with per-router config filtering applied). Only if the
router is too far behind does the controller fall back to a full filtered snapshot.

The shared event cache holds all events; each router only sees a subset, so its index sequence has
gaps. Routers already tolerate index gaps today (not every raft update produces an RDM event), and
`RouterSender` tracks the previous index per router so filtered change sets stitch correctly.

### Link Subsystem Change Notifications

When the router's link subsystem changes (Apply / Remove on `router.link.v1`), the router publishes
its new listener set back to the controller via `ctrl_pb.ContentType_UpdateLinkListenersType
(1056)`. The controller's handler updates `Router.Listeners` and triggers
`RouterMessaging.RouterListenersUpdated`, which reuses the existing `routerChanged` event path to
fan a `PeerStateChange` out to other connected routers.

Dialer changes don't propagate to peers (they only matter locally), but they trigger
`xlinkRegistry.RescanForDialOpportunities()` on the local router — re-evaluating known peer listeners
against the new local dialer set so newly-possible link matches get attempted.

## Applying Configuration on the Router

The router needs to receive config events, route each to the right subsystem, and apply it —
possibly starting, stopping, or reconfiguring subsystems live.

### Local Config Type Allow-list

Not every operator wants the controller to enable arbitrary functionality. A router in a sensitive
environment might need to guarantee that tunneling or edge functionality can never be turned on
remotely. The local config file specifies the allow-list:

```yaml
managedConfig:
  allow:
    - router.link
    - router.forwarder
    - router.xgress.proxy
```

- **Empty or absent** allow-list: controller-managed config is disabled. The router runs purely
  from local config.
- **`all`**: accept every config type from the controller.
- **Explicit list**: only listed types are accepted.

The security boundary sits at the router. The controller can associate whatever configs it wants;
the router has final say on what it applies.

### Precedence: Local Config Wins

If a router has both local and controller-managed config for the same subsystem, local always wins.
Routers are often deployed on systems owned by someone other than the network operator (one entity
runs the network and uses it to manage devices for their customers). The customer needs control
over what the router is capable of. Controller-managed config fills in gaps where local config is
silent; it doesn't override what the operator has explicitly set.

### Source Tracking and Selection

The router-side managed-config registry stores `availableData[source][version]` for each base
type, where source is `SourceController` or `SourceLocal`. Reconciliation picks the effective
config as:

- If `availableData[SourceLocal]` is non-empty: use `max(supportedVersions ∩ localKeys)`.
- Else: use `max(supportedVersions ∩ controllerKeys)`.

Local takes precedence at the **base type level**. If the operator set anything locally for
`router.link`, controller versions are ignored for that base. Matches operator intent: someone who
set v1 locally probably doesn't want the controller silently upgrading them to v2.

Per-field merge — the richest interpretation of "fills in gaps where local config is silent" — is
out of scope for now. Wait for operator demand.

### YAML → JSON Translation

For each managed config type, the YAML loader needs a translator that maps the YAML section to the
controller-equivalent JSON, so locally-loaded config flows through the same handlers as
controller-loaded config. For `router.link.v1`: drop fields removed in the v1 schema (`costTags`,
`split`), rename `dialer.bind` → `dialer.bindInterface`, default `binding` if absent. Per-type
code, lives next to each handler. The registry itself is agnostic.

### Hot Reconfiguration

Every subsystem must handle config updates at runtime: accept new config, tear down and recreate
when needed, shut down when config is removed. Once that machinery exists, it doesn't matter
whether the trigger came from a ctrl event or a local file reload. `ziti agent router
reload-config` can push parsed sections through the same handler registry.

General approach:

- **Additive changes** (new listener, new xgress binding): start the new thing.
- **Removal** (config deleted or section removed): shut down the associated subsystem.
- **Modification**: handler decides whether to update in-place or tear-down-and-recreate.

### Reconciliation Strategy

The link handler takes the simple route: on Apply, build the new listener / dialer set, close the
old listeners, swap in the new state, then `Listen()` on the new listeners. No per-item diffing.

This works without disturbing traffic because `xlink.Listener.Close()` only closes the listener's
accept loop — already-accepted Xlinks survive. So the operator sees:

- **Add a listener**: closes-and-rebuilds, but established links keep running on their original
  listeners until they naturally close.
- **Remove a listener**: same; established links on it stay up.
- **Modify a listener** (advertise change, etc.): the new accept loop is bound with the new
  config; old established links unaffected.
- **No actual change**: the deep-equality check in `notifyChange` no-ops the post-apply notify, but
  the listener rebuild still runs. Cheap enough that we don't optimize for it.

Per-item diffing (a `ReconcileSet[K, V]` helper that fires add/update/remove callbacks) is a
plausible refinement when we have multiple subsystems with item-level identity (xgress listeners,
ctrl-channel listeners), but the link subsystem doesn't need it. Add it when a second consumer
shows up and the simple-rebuild approach has actual cost there.

### Config Application Strategy

The router maintains a config handler registry keyed by config type. When a config event arrives:

1. Check the config type against the local allow-list. Skip if not allowed.
2. Look up the handler.
3. Apply through source-tracked selection (local-wins).
4. Handler validates and applies; rollback runs on failure (below).

For `router.xgress.<binding>` types specifically, the handler looks up the binding in the xgress
registry; if no factory is registered, the config is logged and ignored.

### Config Rollback

On Apply, the registry holds the previously-applied data per handler. The sequence:

1. Try to apply the new config.
2. Success → drop the previous data. Done.
3. Failure → log; try to re-apply the previous data (rollback).
4. Rollback success → subsystem is back to its prior state. Log the rollback so operators see the
   alert pair (apply-failed, rolled-back).
5. Rollback failure → subsystem in unknown state. If it can be shut down (xgress binding, link
   listeners), shut down and log offline. For subsystems that can't meaningfully be shut down
   (forwarder), leave it in whatever state it ended up.

For first-time application with no previous state, failure means the subsystem stays disabled and
gets logged.

### Auto-GC via `gcMode`

`router.link.v1` accepts a top-level `gcMode` field:

- `preserve` (default): never auto-act on stale links.
- `orphaned`: close links that can no longer be re-established at all under the current config.
- `changed`: orphaned's check, plus close links whose re-establishment would produce a different
  link key (peer listener renamed).

The semantic split is **viability vs identity**. "Orphaned" is functional: dialer-side, that means
no `(dialer, remote listener)` pair with the recorded `(binding, protocol)` and overlapping groups
exists. Binding gone, protocol gone, or groups no longer overlapping all count — they make
re-dial impossible regardless of which named piece "went away" first. "Changed" adds an identity
check on top: even if a compatible re-dial would work, the existing link is GC'd if its identity
(`ListenerBinding` in the link key) would drift, so a fresh dial would create a new key rather
than re-using the existing one.

After every Apply that mutates listeners, dialers, or `gcMode`, the router walks its xlink
registry, runs the side-specific staleness check (`CheckDialerSide` / `CheckListenerSide`), and
closes stale entries. Operations are local and one-sided — the router acts on its own verdict
rather than requiring peer agreement, because if the local side can no longer support the link,
the peer was going to see it disconnect anyway. Each closure logs `linkId`, `linkKey`, `side`,
`mode`, and `reason`.

Operators who want a two-sided, controller-aggregated sweep use the `ziti ops verify stale-links`
CLI (which fans CheckStaleLinks out to all routers and only GCs when both endpoints agree).

### Registry Inspection

The router-side managed-config registry exposes an inspect target for diagnostics — answering "why
is this subsystem configured this way?":

- Which handlers are registered (base, supported versions).
- What data is available, by source and version (reported as byte counts, sufficient for "yes it's
  present").
- What's currently applied (source + version).
- Recent alerts (parse failures, rollbacks, offline subsystems).

Wired into the existing inspect framework: `ziti fabric inspect <router> managed-config` returns
the registry view.

### Startup Behavior

1. Router starts with minimal local config (identity, controller endpoints).
2. Router connects to the controller.
3. Controller pushes the router's config set via RDM.
4. Router applies configs and starts subsystems.

Hybrid deployments are typical: local config controls what the operator wants to own directly, and
managed config fills in the rest. Migration from local-only to controller-managed is left to
operators — they have environment-specific patterns and will know which configs should be shared
and where per-router overrides are needed. This is scriptable against the management API.

## Open Questions

1. **Templates / personas.** Routers reference configs by ID, so configs can already be shared.
   What's missing is a way to assign a set of configs to a router in one operation — a named bundle
   ("standard edge router"). Policy/attribute matching is unlikely to work here because it could
   easily reference multiple configs of the same type, which we disallow.

2. **Circuit disruption on config changes.** Some changes require tearing down a subsystem, which
   drops circuits using it. Options: quiesce the router before disruptive changes (drain circuits
   then restart), warn the admin, or invest in making circuits resilient to initiator/terminator
   router restarts. The last is a larger piece of work but pays off broadly.

3. **Config permissions.** Today config + config-type CRUD permissions are unified — anyone who
   can manage service configs can manage router configs. Router config changes affect network
   infrastructure and may warrant higher privilege than service config changes. The `Target` field
   gives a natural split point.
