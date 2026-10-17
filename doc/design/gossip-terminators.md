# Terminators over Gossip

Status: sketch. Nothing here is implemented. This is a holding place for two problems that surfaced
while reviewing the link gossip work on the `gossip-links` branch (link state replication over
gossip, issue #3726) and that both want answering before terminators move onto gossip.

## Why terminators are the next candidate

Links and terminators have the same shape. Both are soft state owned by a single router, both need to
reach every controller because every controller routes, and both are currently reported by every
router to every controller independently. Link state moved to gossip to collapse that
`O(routers × controllers)` fan-out to `O(routers)` (see [gossip.md](gossip.md)); terminators are the
same trade. Where the constant lands depends on the deployment: link count per router is a function
of mesh size, while terminator count is a function of how much a router hosts, and a router fronting
many services can hold far more terminators than it has links.

The store type would follow the pattern the link store already establishes: owner is the hosting
router, key is the terminator id, tombstones on unbind, and the router's own bind table is the source
of truth that anti-entropy reconciles against.

## Prerequisite: routers are only visible to the controllers they connect to

Gossip lets a controller learn about state owned by a router it has no connection to. Nothing else
does, and terminators will run into that sooner than links did.

Today the following does not work, and it is not a gossip problem:

- Router R1 is connected to controller C1 only.
- Router R2 is connected to controller C2 only.
- C1 and C2 are connected to each other.

C1 has nothing at all it can tell R1 about R2:

- **Liveness is local.** `RouterManager.connected` is an in-memory map written by `MarkConnected` on
  that controller's own accept path. No raft command, no gossip type, no cluster event replicates it.
  `routerChangedEvent.handle` builds its entire peer-state update set from `AllConnected()`, so C1
  never mentions R2 to R1 at all.
- **Link listeners are not persisted.** `Router.Listeners`, the addresses R1 would dial to reach R2,
  is deliberately absent from `Router.fillFrom`, so it is never in the database. It is populated only
  from the router's own hello headers at connect time (see `handler_ctrl/accept.go`, which clears it
  and re-reads it from the underlay headers). C1 has no dial address for R2.

Worse than silence: if something does mark R2 as changed on C1, `syncStates` falls through to its
`exists -> Unhealthy` branch, because `GetConnected(R2)` is nil on C1. C1 would actively tell R1 that
R2 is **down** while R2 is perfectly healthy on C2. Any presence mechanism added later has to
displace that, not just fill a gap, or the two will fight.

This works today only because routers connect to every controller. `UpdateControllerDetails` dials
every id in the cluster detail set that is not already usably connected, so each controller's
connected map converges to the whole router set and the per-controller view happens to be the global
one. R1-to-C1-only is not a state the system currently tries to reach.

### Half the machinery already exists

The canary store is already keyed and owned by router id and already replicates to peers, so C1
already receives R2's canary by way of C2. It has live evidence that R2 exists and is ticking; that
evidence is just wired only to epoch and staleness detection, not to peer-state reporting.

What is missing is R2's listeners travelling with it. A `router-presence` store type, owned by the
router and carrying its listeners alongside liveness, would close both halves and would look like
every other store type. That is the shape to aim for, and doing it as part of the terminator work is
reasonable: terminators need the same "learn about a router through a controller it isn't connected
to" property, so the two land together rather than the gap being papered over twice.

### What this does not affect

The destination-affirmation change in the link registry (`linkDest.affirmedBy` /
`linkDest.abandoned`) is safe against this. A destination is only ever dropped for lapsed affirmation
if it was affirmed once and then went unvouched-for; a destination a router never learned about in the
first place cannot be removed, because it was never there. And an established link overrides lapsed
affirmation outright, so a working link is never discarded regardless of controller connectivity.

## Transport terminators do not fit the ownership model

Gossip entries have a single writer: the owner. For terminators that owner is the hosting router,
which is what makes the model work, since the router's bind table is the source of truth and
anti-entropy can re-derive the entry from it.

Transport terminators break that. They are created through the management API against a service with
an address (`checkBinding` defaults an address with no explicit binding to `transport`, or to `udp`
for a `udp:` address), they live in the controller's database, and the router only learns of one when
it is asked to dial it while establishing a circuit. Nobody on the router side would advertise it, and
it has to survive a router restart, because it is configuration rather than runtime state. So it is
not soft state and the router does not own it: exactly inverted from what the store type needs.

### Unifying it through controller-managed router config

Rather than carve out an exception for controller-owned terminators, make transport hosting look like
every other kind of hosting: an xgress implementation on the router, configured by a config that can
optionally come from the controller.

[ctrl-managed-router-config.md](ctrl-managed-router-config.md) already has the pieces:

- Config types follow `router.xgress.<binding-name>`, and the router dispatches on the name by
  stripping the prefix and looking the binding up in the xgress registry. `router.xgress.transport`
  already appears in that document's allow-list example.
- The router keeps an allow-list of config types it will accept, and controller-managed config is
  disabled entirely when the list is absent. Local config wins where both exist.

With transport hosting expressed that way, the router advertises its transport terminators from its
own configuration, the same as it advertises SDK and tunnel terminators from its bind table. One
ownership model, no exception in the store type.

The second benefit is the more interesting one, and it follows from the allow-list rather than from
gossip. Routers frequently sit on systems owned by someone other than the network operator. Today a
network operator can create a transport terminator pointing anywhere reachable from any router, and
the router has no say: it is told to dial an address at circuit time and does. Moving transport
hosting behind a config type the router must accept means the router owner can decline to host
services they do not want originating traffic inside their network. That is the same security
boundary the managed-config allow-list draws for every other capability, applied to a case that
currently has none.

### Open questions

- Existing transport terminators live in the database and are managed through the terminator REST
  endpoints. Whatever the new model is, there needs to be a migration story, and probably a period
  where both work.
- A config-derived terminator has no natural terminator id the way a bind-created one does. It
  probably wants to be derived from the config and the router so it is stable across restarts.
- Precedence and cost are terminator attributes today, set through the API. If the terminator comes
  from router config, do those come from the config too, or stay controller-side? Cost in particular
  is something the network operator may want to keep control of.
- `udp` bindings default the same way `transport` does and have the same ownership problem, so they
  presumably follow along.
