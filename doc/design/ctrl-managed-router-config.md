# Controller-Managed Router Configuration

## The Problem

Today, router configuration lives entirely in a local YAML file. If you want to change how a router behaves,
you edit the file and restart (or in some cases, reload) the router. This is workable for a handful of routers,
but it doesn't scale. It also means there's no central place to define "this is how routers in my network should
behave." You end up with configuration drift, manual toil, and no good story for fleet-wide changes.

What we want is the ability to manage router configuration from the controller. Almost everything should be
manageable this way, with the exception of the controller endpoints, which are already handled by controller
updates pushed to the router.

## What Needs to Be Figured Out

1. How do we split up the configuration?
2. How do we store it?
3. How do we propagate it to routers?
4. How do we apply it on the router side?

## Splitting Up the Configuration

A router's configuration today has several distinct subsystems:

- **Link**: router-to-router link listeners and dialers
- **Xgress bindings**: the points where traffic enters and exits the fabric (proxy, transport, edge, tunnel, etc.)
- **Edge**: SDK connection handling, session management
- **Forwarder**: data plane tuning (worker pools, timeouts)
- **Metrics**: reporting intervals and queue sizes
- **Health checks**: ctrl ping, link checks
- **Transport**: TLS/DTLS configuration
- **Miscellaneous**: profiling, plugins, connect events, interface discovery

Not all of these make sense to manage the same way. Link configuration, for example, is its own concern because
it governs how routers peer with each other. Xgress bindings are per-type because each xgress type (proxy,
transport, edge, tunnel, etc.) has its own factory, its own options, and potentially its own schema.

### Proposed Config Type Breakdown

Each major subsystem gets its own config type:

| Config Type               | What It Covers                                              |
|---------------------------|-------------------------------------------------------------|
| `router.link`             | Link listeners, dialers, heartbeats                         |
| `router.forwarder`        | Forwarding pool sizes, timeouts, rate limiting              |
| `router.metrics`          | Reporting intervals, queue sizes                            |
| `router.healthchecks`     | Ctrl ping check, link check intervals                       |
| `router.transport`        | TLS/DTLS transport-level settings                           |
| `router.xgress.<binding>` | Per-xgress-type configuration (one per binding type)        |

The `router.xgress.<binding>` pattern deserves some explanation. Each xgress binding (proxy, transport,
transport_udp, edge, tunnel, etc.) is backed by a factory that creates dialers and/or listeners. Different
bindings have different options. Rather than trying to stuff all xgress config into one type, we let each
binding define its own config type. A config named `router.xgress.proxy` would contain the options relevant
to the proxy xgress factory.

The tunnel binding already works this way: its configuration is loaded entirely through xgress listener
options, with no top-level config section. Edge currently has its own top-level `Edge` config in the router
YAML, but we'd consolidate that into `router.xgress.edge` so that all xgress bindings follow the same
pattern. This means edge configuration (SDK listeners, session management, etc.) lives alongside the other
xgress options for that binding.

### Config as a Feature Toggle

One useful side-effect of this approach: the presence or absence of a config can drive whether a subsystem
is enabled. If a router has a `router.xgress.tunnel` config, the tunneler is active. Remove it, and the
tunneler shuts down. Same for `router.xgress.edge`. This gives operators a clean, declarative way to enable
and disable router capabilities from the controller, and there's exactly one place to look to see whether
a given binding is on or off.

## Storage

### Reusing the Config Type Infrastructure

We already have a config type system: `ConfigType` defines a schema, `Config` holds an instance of that type's
data, validated against the schema. Today this is used for service configs (intercept, host, etc.). We can
reuse the same infrastructure for router configs.

What's missing is a way to distinguish "this config type is meant for a service" from "this config type is
meant for a router." We can fix that by adding a **target** field to `ConfigType`:

```go
type ConfigType struct {
    boltz.BaseExtEntity
    Name   string                 `json:"name"`
    Schema map[string]interface{} `json:"schema"`
    Target string                 `json:"target"` // "service", "router", etc.
}
```

Possible target values:

- `service` (default, for backward compatibility): used by SDK identities, associated with services
- `router`: consumed by routers, associated with routers

This is helpful for UIs. A router management screen can query config types where `target = "router"` and
only show those. A service config screen shows `target = "service"`. It also lets the API enforce that you
don't accidentally associate a tunneler intercept config with a router, or a router link config with a service.

### Associating Configs with Routers

Today, services have a `Configs []string` field that holds config IDs. We'd add the same thing to the router
model:

```go
type EdgeRouter struct {
    // ... existing fields
    Configs []string `json:"configs"` // Router config IDs
}
```

Like services, a router can have at most one config per config type. This is enforced at the store level.

### Handling Arbitrary Xgress Types

A question that comes up: what about custom xgress types that aren't defined on the controller? Third-party
xgress implementations might need their own configuration, and the controller doesn't necessarily know about
them in advance.

The approach: anyone who needs a custom xgress config can create their own config type, set `target = "router"`,
and follow the `router.xgress.<binding-name>` naming convention. The router uses the config type name to
dispatch: it strips the `router.xgress.` prefix to get the binding name, then looks that up in the xgress
registry. If the factory exists, it starts it up with the provided configuration. If not, it logs a warning
and moves on. No additional metadata is needed on the config type itself.

## Propagation

### Distributing Routers in the RDM

Today the Router Data Model distributes identities, services, service policies, configs, config types,
posture checks, public keys, and revocations. Routers themselves are not in the RDM. The controller
communicates router peer state separately via `PeerStateChanges` messages.

A natural next step is to add routers as a first-class entity in the RDM. This gets us two things:

1. **Router fingerprints for link validation.** If every router knows the fingerprints of other routers
   via the RDM, it can validate the identity of peers when establishing links. Today link validation
   relies on the certificate chain, but distributing fingerprints gives us an additional check that
   the controller has explicitly authorized that router.

2. **A vehicle for router config distribution.** Once routers are in the RDM, we can attach configs to
   them and distribute those configs through the same event/replay infrastructure that already handles
   services and identities.

The `DataState` protobuf would get a new `Router` message:

```protobuf
message DataState {
    // ... existing fields

    message Router {
        string id = 1;
        string name = 2;
        string fingerprint = 3;
        repeated string configs = 4;  // config IDs associated with this router
    }

    message Event {
        // ... existing oneof options
        Router router = <next_field_number>;
    }
}
```

### Selective Config Distribution

The current RDM is a single shared `RouterDataModelSender` that broadcasts everything to every router.
All routers get the same identities, services, policies, etc. For router configs, that's not ideal:
a router's config may contain binding addresses, credentials, or tuning parameters that are only
relevant to that specific router.

It would be interesting if we could distribute router configs only to the routers they apply to. There
are a few ways to approach this:

**Option A: Per-router filtering at send time.** The `RouterSender` (which wraps each router's connection
to the shared RDM) could filter events before sending. When building a full `DataState` snapshot or
replaying change sets, it would include only the `Router` and `Config` events relevant to that specific
router. The shared `RouterDataModelSender` still stores everything, but each `RouterSender` acts as
a filter. This keeps the single-RDM architecture intact while adding per-router scoping.

**Option B: Separate per-router config channel.** Router configs could be sent outside the RDM entirely,
as direct messages over the ctrl channel when a router connects or when its config changes. This is
simpler but loses the replay/gap-detection guarantees of the RDM event cache.

**Option C: Per-router RDM overlays.** Each router gets the shared RDM plus a small per-router overlay
containing just its own router entity and configs. The overlay would have its own event index and replay
cache. This is the most complex option but gives clean separation.

Option A is probably the right starting point. The filtering is straightforward: when building the
`DataState` or replaying events, include `Router` events for all routers (everyone needs fingerprints for
link validation), but include `Config` events only when the config is associated with the receiving router.
The `RouterSender` already has access to the router's ID, so the filter is just a membership check on the
config's associated router set.

### Change Notification Flow

1. Operator creates/updates/deletes a config associated with a router.
2. Controller generates a `DataState.Event` with the config change at the next index.
3. When broadcasting to connected routers, the `RouterSender` checks whether each config event is
   relevant to its router. If so, it includes it. If not, it skips it.
4. For full syncs on connect, the `RouterSender` builds a filtered `DataState` snapshot that includes
   only configs associated with that router.
5. Router entities themselves (id, name, fingerprint, cost) are distributed to all routers, so every
   router can validate link peers.

This means the shared event cache contains all events, but each router only sees a subset. The index
sequence will have gaps from the router's perspective, but this is already the case today: not every
raft update produces an RDM event, so routers already tolerate index gaps. We may need to move some
of the gap-handling logic into the `RouterSender` so it can track the previous index per router and
set it correctly on filtered change sets, rather than relying on the receiver to reconcile.

## Applying Configuration on the Router

This is where things get interesting. The router needs to:

1. Receive config events from the controller.
2. Determine which subsystem each config belongs to.
3. Apply the configuration, potentially starting, stopping, or reconfiguring subsystems.

### Config Application Strategy

The router maintains a config handler registry, keyed by config type name (or pattern). When a config event
arrives:

1. Look up the handler for the config type.
2. If found, pass the config data to the handler.
3. The handler validates and applies the config, returning any errors.

For xgress configs specifically, the flow would be:

1. Router receives a `router.xgress.proxy` config.
2. It recognizes the `router.xgress.*` pattern and looks up the `proxy` binding in the xgress registry.
3. If the factory exists, it creates/reconfigures the listener and/or dialer with the new options.
4. If the factory doesn't exist (custom binding not loaded), it logs a warning.

### Hot Reconfiguration vs. Restart

Some config changes can be applied at runtime (metrics intervals, health check timings, forwarder pool sizes).
Others may require tearing down and recreating subsystems (changing a link listener's bind address, for
example). Each config handler should know whether it can apply a change in-place or needs a restart of
its subsystem.

A reasonable approach:

- **Additive changes** (new listener, new xgress binding): start the new thing.
- **Removal** (config deleted): shut down the associated subsystem.
- **Modification**: depends on the subsystem. Some can hot-reload, some need a restart cycle. The handler
  decides.

### Startup Behavior

On startup, before the router had a local YAML config that drove initialization. With controller-managed
config, the flow becomes:

1. Router starts with minimal local config (identity, controller endpoints).
2. Router connects to the controller.
3. Controller pushes the router's config set.
4. Router applies configs and starts subsystems.

There's a bootstrapping question here: the router needs its identity and controller endpoint to connect
in the first place. Those stay in the local config. Everything else can come from the controller.

We may also want to support a hybrid mode during migration, where local config is used as a fallback if
the controller hasn't pushed config yet, or as a baseline that controller config overrides.

## Open Questions

1. **Precedence**: If a router has both local config and controller-managed config for the same subsystem,
   which wins? Proposal: controller config wins, local config is fallback for subsystems not managed by
   the controller.

2. **Migration path**: How do we move existing routers from local-only config to controller-managed config?
   We probably need a CLI command or API endpoint that imports a router's current YAML config into the
   appropriate config type instances.

3. **Validation**: Config types support JSON Schema validation. We should define schemas for each built-in
   router config type so the controller can reject invalid config before it ever reaches a router.

4. **Versioning**: Router software versions may not support all config fields. Do we need a way to express
   "this config field requires router version >= X"?

5. **Audit trail**: Config changes to routers should be auditable. The existing event infrastructure should
   cover this, but we should verify.

6. **Rollback**: If a config change causes a router to malfunction, what's the recovery path? Should the
   router report back that a config application failed, and if so, what does the controller do with that
   information?

7. **Bulk operations**: We should support applying a config to multiple routers at once, likely via role
   attributes and policies (similar to how service policies work today).
