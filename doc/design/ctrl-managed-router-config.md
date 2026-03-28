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

All built-in router config types will have JSON Schema definitions so the controller can reject invalid
config before it ever reaches a router.

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

### Versioning

Config type names include a version suffix: `router.link.v1`, `router.xgress.edge.v1`, etc. The
versioning rules are designed so that old routers and new configs can coexist gracefully:

1. **Additive only within a version.** New fields can be added to a config version at any time.
   Older routers that don't know about the new fields will ignore them.
2. **No removals or semantic changes within a version.** Fields are never removed, and their
   meaning never changes. A field can be marked deprecated, but it continues to work as it
   always did.
3. **Breaking changes require a new version.** If we need to remove a field, change its meaning,
   or restructure the config, we create a new version: `router.link.v1` becomes `router.link.v2`.
4. **Routers select the highest version they understand.** A router knows which config versions it
   supports. When it receives its config set, it looks for configs from highest version to lowest
   and uses the first match. A v3 router with both `router.link.v1` and `router.link.v2` configs
   associated will use v2.
5. **Internally, the router uses a single config format.** Regardless of which version the config
   came from, the router populates its internal config struct from the highest-version config it
   understands. This is the same pattern we use today for host configs.
6. **Forward-compatible field additions.** If something that was a single value needs to become a
   list, we add a new field for the list. The router checks for the list field first and falls
   back to the single-value field if not present. No version bump needed.

This approach gives operators a clean path for rolling upgrades. During a fleet upgrade, both
`router.link.v1` and `router.link.v2` can be associated with the same router. Old routers pick
v1, upgraded routers pick v2. Once the fleet is fully upgraded, v1 can be removed.

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

### Local Config Type Allow-list

Not every router operator will want the controller to be able to enable arbitrary functionality. For
example, a router deployed in a sensitive environment might need to guarantee that tunneling or edge
functionality can never be turned on remotely. To support this, the router's local config file can
specify which config types it will accept from the controller:

```yaml
managedConfig:
  allow:
    - router.link
    - router.forwarder
    - router.xgress.proxy
    - router.xgress.transport
```

The allow-list rules:

- If the allow-list is **empty or absent**, controller-managed config is **disabled entirely**. The
  router operates purely from its local config file, same as today.
- The special value `all` accepts every config type from the controller.
- Otherwise, only the listed config types are accepted.

This keeps the security boundary at the router. The controller can associate whatever configs it
wants with a router, but the router has the final say on what it actually applies. An operator who
doesn't want edge or tunnel functionality enabled remotely simply omits `router.xgress.edge` and
`router.xgress.tunnel` from the allow-list. An operator who wants full controller management uses
`all`. And existing routers with no `managedConfig` section continue to work exactly as they do
today.

### Precedence: Local Config Wins

If a router has both local config and controller-managed config for the same subsystem, local config
always wins. The router operator has final authority.

This follows from the same principle as the allow-list. Routers are often deployed on systems owned
by someone other than the network operator. It's common for one entity to run the network and use
it to manage devices or software for their customers. The routers sit on customer systems, and the
customer needs to control what the router is capable of and prevent capabilities from being enabled
that don't make sense for their environment. Controller-managed config fills in gaps where local
config is silent, it doesn't override what the operator has explicitly set.

### Config Application Strategy

The router maintains a config handler registry, keyed by config type name (or pattern). When a config event
arrives:

1. Check the config type against the local allow-list. If not allowed, skip it.
2. Look up the handler for the config type.
3. If found, pass the config data to the handler.
4. The handler validates and applies the config, returning any errors.

For xgress configs specifically, the flow would be:

1. Router receives a `router.xgress.proxy` config.
2. It checks the allow-list. If `router.xgress.proxy` is not allowed, the config is ignored.
3. It recognizes the `router.xgress.*` pattern and looks up the `proxy` binding in the xgress registry.
4. If the factory exists, it creates/reconfigures the listener and/or dialer with the new options.
5. If the factory doesn't exist (custom binding not loaded), it logs a warning.

### Hot Reconfiguration

For controller-managed config to work, every subsystem needs to handle config updates at runtime:
accepting new config, tearing down and recreating if needed, or shutting down when config is removed.
Once that machinery exists, there's no reason it should only work for configs arriving over the ctrl
channel. The same handlers can process config from a local file reload.

This means we get hot-reloading of the local config file as a natural side-effect of this work. A
`ziti agent router reload-config` command can trigger a re-read of the local config file and push the
parsed sections through the same config handler registry that processes controller-managed updates.
No restart required.

The general approach for config handlers:

- **Additive changes** (new listener, new xgress binding): start the new thing.
- **Removal** (config deleted or section removed): shut down the associated subsystem.
- **Modification**: depends on the subsystem. Some can update in-place, some need a tear-down and
  recreate cycle. The handler decides.

### Startup Behavior

On startup, before the router had a local YAML config that drove initialization. With controller-managed
config, the flow becomes:

1. Router starts with minimal local config (identity, controller endpoints).
2. Router connects to the controller.
3. Controller pushes the router's config set.
4. Router applies configs and starts subsystems.

There's a bootstrapping question here: the router needs its identity and controller endpoint to connect
in the first place. Those stay in the local config. Everything else can come from the controller.

In practice, most deployments will run in a hybrid mode: local config defines whatever the operator
wants to control directly, and controller-managed config fills in the rest. Local config always takes
precedence, so there's no conflict. Migration from local-only to controller-managed config is left to
network operators. They generally have patterns specific to their environment and will know best which
configs should be shared across routers and where they need per-router overrides. This can be scripted
against the management API.

## Consolidating Router Types

### Current State

The controller has three router types in a hierarchy:

- **Router** (base): name, fingerprint, cost, noTraversal, disabled, ctrlChanListeners, interfaces.
- **Edge Router** (extends Router): adds roleAttributes, isVerified, certPem, isTunnelerEnabled,
  appData, unverifiedCertPem, unverifiedFingerprint. Enrolled via `erott`. Participates in
  EdgeRouterPolicy and ServiceEdgeRouterPolicy for access control.
- **Transit Router** (extends Router): adds isVerified, isBase, unverifiedFingerprint,
  unverifiedCertPem. Enrolled via `trott`. No role attributes, no policy participation.

Edge Router and Transi tRouter are stored as child entities of Router in separate database buckets
(`edge` and `transitRouter` respectively). They have separate API endpoints (`/edge-routers` and
`/transit-routers`), separate managers, and separate enrollment methods.

The `IsTunnelerEnabled` flag on Edge Router triggers auto-creation of a matching Identity
(type="Router", same ID as the router) and a system Edge Router Policy that links the identity to
the router. This gives the tunneler-enabled router an identity it can use to authenticate as a
client and access services.

The `IsBase` flag on Transit Router is inferred at load time when a transit router has no child
bucket. It marks legacy routers that predate the transit router store and prevents updates.

### What Each Type Actually Provides

When you strip away the naming, the differences boil down to:

- **Transit Router** = Router + enrollment mechanism. That's it. No role attributes, no policies, no
  tunneling. It exists to give fabric-only routers a way to enroll.
- **Edge Router** = Router + enrollment + role attributes + policy participation + optional tunneler
  identity. The edge-specific parts are what let the router participate in the policy model and
  optionally act as a tunneler.

With controller-managed config, the distinction between "edge" and "transit" becomes less meaningful.
Whether a router runs the edge subsystem is determined by whether it has a `router.xgress.edge`
config. Whether it tunnels is determined by `router.xgress.tunnel`. The router type in the data
model shouldn't need to dictate capabilities.

### Collapsing to a Single Router Type

We can fold Edge Router and Transit Router down into the base Router type. The base Router would
absorb the fields it needs:

```go
type Router struct {
    boltz.BaseExtEntity
    Name                  string
    Fingerprint           *string
    Cost                  uint16
    NoTraversal           bool
    Disabled              bool
    CtrlChanListeners     map[string][]string
    Interfaces            []*Interface

    // From EdgeRouter
    RoleAttributes        []string
    AppData               map[string]interface{}
    Configs               []string               // new, for managed config

    // From EdgeRouter/TransitRouter (enrollment)
    IsVerified            bool
    CertPem               *string
    UnverifiedFingerprint *string
    UnverifiedCertPem     *string
}
```

A single enrollment method replaces `erott` and `trott`. The API surface collapses to `/routers`.
The separate EdgeRouterPolicy and ServiceEdgeRouterPolicy types would reference routers directly
(or we introduce unified router policy types).

### The Identity Question

Today only tunneler-enabled edge routers get an identity. With a unified router type, we have a
choice:

**Option A: Identity tied to tunnel config.** When a `router.xgress.tunnel` config is associated
with a router, the system auto-creates the matching identity and system policy, same as the
`IsTunnelerEnabled` constraint does today. When the config is removed, the identity and policy are
cleaned up. This preserves the current behavior, just driven by config presence instead of a boolean
flag.

**Option B: Every router gets an identity.** If every router has an identity, it simplifies the
model. Routers can always authenticate as themselves, appear in the identity list, and participate
in policies uniformly. The tunneler just happens to be one consumer of that identity. This also
opens the door for other use cases, for example, a router that needs to authenticate to external
systems using its Ziti identity.

Option B is cleaner long-term but has implications: every router creation would also create an
identity, the system edge router policy would always exist, and existing transit routers would need
identities created during migration. It also means the router identity exists even if the router
never uses it, which is a small cost.

### System Edge Router Policy

Currently the system EdgeRouterPolicy is auto-created per tunneler-enabled router, linking the
router's identity to the router itself so the tunneler can access services through its own router.

With a unified model:

- If we go with **Option A**, the policy is created/deleted when `router.xgress.tunnel` config is
  added/removed. The trigger moves from the `IsTunnelerEnabled` db constraint to a config change
  handler.
- If we go with **Option B**, every router gets the system policy at creation time. The policy
  exists whether or not the router is tunneling. This is simpler but means every router has a
  policy entry even if it's not needed.

### Migration

Collapsing the types requires a database migration:

1. Move EdgeRouter-specific fields (roleAttributes, appData, certPem, etc.) into the base Router
   bucket.
2. Move TransitRouter-specific fields (isVerified, unverifiedFingerprint, etc.) into the base
   Router bucket.
3. Remove the `edge` and `transitRouter` child buckets.
4. Consolidate enrollment records to use a single method.
5. If going with Option B, create identities and system policies for all existing routers that
   don't already have them.

The API migration is trickier. The `/edge-routers` and `/transit-routers` endpoints would need to
be deprecated in favor of `/routers`. We could keep the old endpoints as aliases for a transition
period.

### Config Rollback

When applying a config update, the router follows this sequence:

1. Hold on to the previous config before applying the update.
2. Try to apply the new config.
3. If it succeeds, drop the previous config. Done.
4. If it fails, generate an alert to the controller.
5. Try to re-apply the previous config.
6. If the rollback succeeds, done. The subsystem is back to its prior state.
7. If the rollback also fails, generate another alert to the controller. At this point the
   subsystem is in an unknown state. If the subsystem can be shut down (xgress bindings, link
   listeners, etc.), shut it down and alert that it's now offline. For subsystems that can't
   meaningfully be shut down, like the forwarder where config changes are just adjusting defaults,
   the router has to leave it in whatever state it ended up in. In practice, those subsystems are
   also the ones least likely to fail a config application.

For first-time config application where there is no previous config to roll back to, a failure
simply means the subsystem stays disabled and an alert is sent.

## Open Questions

1. **Templates/personas**: Since routers reference configs by ID, a single config can already be
   shared across multiple routers. What we don't have yet is a way to assign a set of configs to
   a router in one operation. Something like a router template or persona, a named bundle of
   config references, would let an operator say "this router is a standard edge router" and have
   it pick up the full set of configs associated with that template. The policy/attribute approach
   is unlikely to work well here because you could easily end up referencing multiple configs of
   the same type, which is not allowed.
