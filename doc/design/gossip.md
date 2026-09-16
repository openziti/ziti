# Gossip-Based Link State Distribution

## What Problem Are We Solving?

In an HA controller cluster, link state needs to reach all controllers quickly.
The old approach had every router report links to every controller independently
producing `O(routers × controllers)` messages. With 400 routers and 3 controllers,
that's a lot of redundant traffic for data that's inherently soft state.

Links are ephemeral. If a controller loses track of a link, the router can just
re-report it. We don't need Raft consensus or durable storage. We need fast,
eventually-consistent distribution with good failure recovery.

## How It Works

Each router picks a **subscription controller** (preferred: the Raft leader,
fallback: the most responsive connected controller) and sends gossip deltas
only to that one controller. The subscription controller broadcasts to peers.
This reduces link reporting traffic to `O(routers)`.

The system has three layers:

| Layer | What it does |
|---|---|
| **Gossip deltas** | Router → subscription controller → peer broadcast. The hot path. |
| **Anti-entropy** | Periodic controller-to-controller digest exchange. The consistency backstop. |
| **Canary protocol** | Router sends heartbeat sequence to detect gossip lag and router restarts. |

## The Gossip Store

Each controller maintains an in-memory gossip store with typed state maps. Each
entry has:

```
Key       string   // e.g., "linkId:iteration"
Value     []byte   // serialized link proto
Version   uint64   // Lamport clock from the originating router
Owner     string   // router ID, the single writer
Epoch     []byte   // 16-byte UUIDv7, identifies the router's lifetime
Tombstone bool     // marks a deleted entry
```

Version-based merge: if incoming version > local version, accept. Equal or less,
ignore. Simple, no conflicts (each entry has a single writer).

The single-writer property holds for entries a router gossips itself. Entries
translated from a legacy router's report are owned by the router but versioned by
a controller's clock. Legacy reports are kept out of gossip for that reason. See
[Legacy Link Reporting](#legacy-link-reporting).

### Epochs

Every router generates a fresh epoch (UUIDv7, raw 16 bytes) on startup. The
epoch is included on every gossip entry and in the canary. UUIDv7 encodes a
millisecond timestamp in the high bits, so raw bytes sort chronologically via
`bytes.Compare`. A newer epoch always compares greater than an older one.

When a controller detects a new epoch for a router (via canary or the router's
hello headers), it deletes all link gossip entries from the old epoch. The
router then re-populates with fresh entries. This eliminates the class of bugs
where a router restart resets its Lamport clock and old entries become
indistinguishable from new ones.

### Tombstones

When a link faults, the router sends a tombstone: a gossip entry with
`Tombstone=true`. The controller replaces the live entry with the tombstone,
removes the link from its link manager, and broadcasts to peers. Tombstones are
reaped after a configurable TTL (currently 5 minutes).

### State Types

The gossip store is generic. Two state types are registered:

- **`links`**: link state entries with tombstones and anti-entropy enabled.
  Owner is the dialing router. Listener creates/removes links in the link
  manager.

- **`canaries`**: canary probes with no tombstones and no anti-entropy.
  Fire-and-forget. Carries `{Seq uint64, Epoch []byte}` per router. The
  listener detects epoch changes and triggers old-epoch link cleanup.

## The Canary Protocol

The canary does double duty: gossip health monitoring and epoch distribution.

Every 5 seconds, the router sends a canary message to its subscription
controller containing a monotonically increasing sequence number and the
router's epoch. The controller stores it in the canary gossip store and
broadcasts to peers. Each controller periodically sends its view of the
router's canary sequence back to the router.

The router compares the returned sequences across all connected controllers. If
a non-subscription controller's sequence lags by ≥ 6 ticks (30 seconds), the
router marks it as **stale** and:

1. Sends a `GossipDigestRequest` to trigger a catch-up digest exchange.
2. Dual-sends subsequent gossip deltas to both the subscription controller and
   the stale controller until it recovers.

If the subscription controller changes (leader election, reconnect), the router
detects the change and triggers a digest exchange with the new primary.

## Anti-Entropy

Controllers periodically exchange digests to repair missed broadcasts. The
sender picks a rotating peer every 30 seconds and sends a digest: one FNV-64a
hash per owner, covering that owner's live entries and tombstones. The peer
compares each of its own owners against the hash it was sent and responds with:

- nothing, for owners whose hash matches (identical on both sides)
- every entry it holds for the owner, when the hash differs or the sender
  listed no hash for that owner at all

The digest deliberately carries no per-entry versions. In a full mesh every
router owns an entry per link, so a `(key, version)` digest is a multi-megabyte
message every interval, almost always concluding that nothing has diverged. The
trade is precision within a diverged owner: the peer re-sends that owner's whole
set, and the redundant entries lose their version check on arrival. That
redundancy is bounded by one owner's entries and is counted separately, under
`gossip.<type>.anti_entropy.entries.*`, so it does not disturb the
broadcast path's `delta.rejected_stale` rate.

A peer still running the older exchange sends per-entry versions as well; where
it does, the response is narrowed to just the entries it is behind on.

An empty digest is meaningful and is still sent: it says the sender holds
nothing, which is what a restarted controller needs to advertise in order to be
sent everything.

Anti-entropy is the consistency backstop. In normal operation, broadcasts
deliver everything. Anti-entropy catches anything that slipped through during
transient disconnections.

## Router Reconnect Digest Exchange

When a router connects to a controller, the controller sends a digest of all
gossip entries owned by that router. The router compares with its current link
state and responds with corrections:

- Entries the router has but the controller doesn't → sent as live entries
- Entries the controller has but the router doesn't → sent as tombstones

Before creating tombstones, the router advances its Lamport clock past all
versions in the digest (the `observeVersion` call). This ensures tombstones
have higher versions than the entries they replace, even after a restart resets
the clock.

## Disconnect Handling

### Router Disconnects from a Controller (HA Mode)

The controller marks all links where the disconnected router is the source as
"down" but does NOT remove them. The router may still be connected to other
controllers. When the router reconnects, `ConnectRouter` marks the links as
usable again and runs the gossip link reconciliation to update stale router
pointers.

### Router Restart

The router generates a new epoch. When the canary (or hello header) carries the
new epoch to each controller, the controller bulk-deletes all old-epoch link
entries. The router re-populates via normal gossip deltas.

### Controller Restart

The controller starts with an empty gossip store. `PeerConnected` snapshots
from existing peers bootstrap its state. The router reconnect digest exchange
handles any entries the peers missed.

## Link Lifecycle (Gossip Perspective)

1. Router dials link → `DialSucceeded` → link registered
2. `notifyControllersOfLinks` (every 5s) → `NotifyLinks()` → gossip delta to
   subscription controller with epoch
3. Controller applies via `ApplyAndBroadcast` → listener creates link in link
   manager → broadcasts to peers
4. Link faults → `NotifyLinkFault()` → tombstone gossip delta
5. Controller listener removes link, broadcasts tombstone to peers
6. After tombstone TTL → reaped from gossip store

## Duplicate Link Detection

When both sides of a link pair dial simultaneously, duplicate detection in
`applyLink` resolves the conflict by lexicographic comparison of link IDs.
Before entering the full `DialSucceeded` path, the dial worker checks if a link
already exists for this key. If the existing link's ID is lower (it would win),
the dialed link is closed early to avoid the heavier duplicate resolution
cascade.

## Mixed-Version Operation

Gossip is negotiated, not assumed. A router uses the gossip path only when
`AllControllersHaveCapability(capabilities.ControllerLinkGossip)` holds, which
returns false if any *connected* controller lacks the capability and false if
none are connected. It fails closed, but note that it tracks connectivity, not
deployment: a router's mode flips when it loses or gains a controller
connection, not only when controllers are upgraded.

Routers that don't gossip fall back to the legacy `RouterLinks` report, which
every controller they're connected to receives directly.

### The invariant

**Every controller knows every link, in every version mix.** This holds without
gossip: the legacy report broadcasts to all connected controllers, so in steady
state with full connectivity each controller learns each link directly. Gossip's
value is recovery from partitions and reconnects, not steady-state distribution.

So there is one oracle, not a per-phase weakening. Divergence between
controllers is always a failure, whatever the version mix.

### Test matrix

Routers run 1/3 each at 2.1, 2.0 and 1.6. Links are pairs, so thirds generate
all six version combinations naturally (1.6<->1.6, 1.6<->2.0, 1.6<->2.1,
2.0<->2.0, 2.0<->2.1, 2.1<->2.1). A 50/50 split would miss the middle version's
interactions. There is no minimum-router-version gate on connect, so 1.6 routers
attach normally.

Controllers cycle through four phases:

| Phase | Controllers | `AllControllersHaveCapability` | 2.1 routers use |
|---|---|---|---|
| 1 | 3x 2.0 | false | legacy |
| 2 | 1x 2.1, 2x 2.0 | false | legacy |
| 3 | 3x 2.1 | **true** | gossip |
| 4 | 2x 2.1, 1x 2.0 | false | legacy |

Phases 2->3 and 3->4 walk the capability boundary in both directions. 1.6
controllers are out of scope: that release is no longer supported for
controllers.

Mixed *controller* versions are a transient upgrade state in practice, so the
phases are about correctness during the window rather than a supported steady
state. Mixed *router* versions are the real long-lived case.

### Open design points

- **Transitions vs settled state.** The interesting bugs live in the transitions: a 2.1 router losing
  gossip at 3->4 forgets its advertised set and starts reporting the pre-gossip way, and whether the
  controllers give up the matching entries is exactly what this should catch. Asserting mid-transition is
  harder, so start with settled-state assertions and add transition checks once the phases work.
- **Chaos during phase changes.** Running partitions through the transitions is what produces a router
  connected to only part of the cluster, which is the case the cluster-wide capability answer exists for. It
  also makes failures harder to attribute, so probably not on the first run.
- **Which routers gossip in each phase.** Only phase 3 has gossip usable at all, so phases 1, 2 and 4 are
  testing that the legacy path still carries everything and that nothing is left behind in a gossip store.

Leadership no longer needs controlling. It decided which controller translated legacy reports into gossip,
and nothing translates them now.

## Legacy Link Reporting

Routers that do not gossip report links the way 2.0 did, and those reports stay out of the
gossip store entirely. Gossip carries link state for gossip-capable routers only.

### What this replaces

`routerLinkHandler.HandleLinks` originally translated legacy reports into gossip entries, gated
on `network.IsLeader()`. That gate existed because of a modelling mismatch: the translated entry
was owned by the **router** (`NotifyLinkViaGossip` passes `srcRouter.Id`) but versioned from the
**controller's** clock (`stateMap.set` -> `Store.nextVersion`). One owner, N clocks, so the leader
was made the only writer.

The gate cost information. With router R connected to controllers A and B but not to leader C, A
and B learned R's links directly and neither gossiped them, so C never heard about them at all.
Two controllers held the state, a replication mesh existed, and it went unused.

Letting every controller write instead would have made the mismatch worse rather than better,
since `Reconcile` is owner-scoped and two controllers with divergent views of R would tombstone
each other's fresh entries and then re-mint above them. Keeping legacy reports out of gossip
removes the mismatch instead of managing it.

### Why the two mechanisms do not overlap

Ownership is by dialer in both protocols, so the link set partitions cleanly.

A legacy report only ever covers links the reporting router dialed. Both send paths filter on
`link.IsDialed()`: the incremental one at `link_registry.go:873` and the full refresh at
`link_registry.go:484`. Connection state follows the same rule, as
`linkRegistryImpl.syncRequiredLinkStates` notes: "Only the end that dialed a link reports its
connection state, matching every other path that sends it."

So every link has exactly one publisher, its dialer, and that dialer either gossips or reports
legacy. No link is described by both mechanisms, which is what would make two removal regimes
dangerous.

Both directions converge without gossip:

- **Creation** broadcasts. A legacy report goes to every controller the router is connected to.
- **Removal** broadcasts too. `linkRegistryImpl.sendLinkFaults` in the non-gossip branch sends the
  `Fault` to every connected controller and only marks it sent once all of them took it.

The invariant that every controller knows every link therefore holds through both paths, which is
what it did in 2.0.

### No source marker is needed

Each removal path is already scoped to its own kind by construction:

- `linksToPrune` filters on `link.Src.Id == routerId`, and that router is legacy, so all of its
  dialed links are legacy.
- Gossip removal is driven by gossip entries, which exist only for gossip-capable routers.

Validation does need to know which mechanism a router uses, so it can skip the gossip checks for a
legacy router. It derives that from the router's version rather than from a per-link marker. That
awareness is required for mixed-version testing regardless, since a 1.6 router has no local gossip
store and `validateGossip`'s `inLocalGossip` check is already meaningless for it.

### What the legacy path costs

A legacy link's fault reaches the peer router only when that peer happens to be connected to the
same controller. Gossip propagates a tombstone to every controller, each of which forwards a
`LinkFault` to whichever endpoint it holds; a broadcast fault removes the link everywhere but
carries no such forwarding. This is the 2.0 behaviour for 2.0 routers, and the router's own
failure detection closes the gap, just more slowly.

Separately, a duplicate-link fault loses its distinct event on the gossip path. `FaultSubject_LinkDuplicate`
and `FaultSubject_LinkFault` both reach `faultHandler.handleFaultedLink`, and the pre-gossip path passed the
distinction to `Network.LinkFaulted` so it raised a `LinkDuplicate` event. A tombstone carries no such
flag, so a gossiped link is always reported as a plain fault. Links that are not in gossip keep the
distinction, since they take the pre-gossip path unchanged; closing it for gossiped links needs the flag on
the entry, which is a separate change.

### Deciding whether gossip is usable

Two answers have to hold before a router publishes over gossip, and each party is the only one that can
give its own.

**The router's own view.** `NetworkControllers.AllControllersHaveCapability` folds over the controllers this
router is connected to. It asks whether everything the router is talking to can hear it, which no controller
can see.

**The cluster's view.** `Controller.deriveClusterCapabilities` intersects the capability masks of every raft
member. It asks whether the feature can be carried at all. A router connected only to capable controllers
would otherwise keep publishing while a member it never connects to holds the feature back, and only the
controllers can see that member.

Controllers report their answer as a mask on `ClusterCapabilitiesHeader`, carried by the canary status
message that already flows periodically to each router. A mask rather than a boolean, so a release that adds
a second gossip feature can say "links yes, terminators no" to a router that only knows about the first: it
reads the bit it knows and ignores the rest. `NetworkControllers.UsesGossip` is the conjunction, and every
gossip decision on the router goes through it.

The cluster answer is stable where a per-connection one is not. It comes from raft membership rather than
from who happens to be connected, so every controller gives the same answer and a router losing a connection
cannot change it. Capabilities are remembered per member rather than read from the connected peers, since
the mesh's peer map holds only connected ones; a member cannot change what it advertises without
reconnecting, so its last answer holds until it does.

Two conservative defaults, pointing opposite ways on purpose:

- A raft member nothing has ever been seen from narrows the cluster mask to nothing. Clustered is
  conservative.
- A router that no controller has reported to treats every capability as supported. Standalone is
  permissive, since a controller with no peers has no member that could fail to take part.

### Giving up a capability

When a capability stops being usable, both sides stop, and neither deletes what the entries describe.

**Controllers** discard the affected store types: entries dropped without notifying listeners and without
telling peers. The links themselves stay in the link manager, to be reported the pre-gossip way. Tombstoning
instead would remove them from every controller, and would need versions minted from a controller's clock
for entries a router owns. Anti-entropy stops for those types, and inbound entries are refused on all three
apply paths, or a peer that has not noticed would refill a store that was just given up.

**Routers** forget their advertised set for the affected stores. Nothing is sent: every controller that could
hold an entry has given up its store on reaching the same conclusion, so there is nobody to tell. The forget
is conditional on the version each entry was read at, since publishing runs on another goroutine and can
re-advertise a link in between. A router also stops answering digests for those types, since the exchange
retires whatever the controller holds that the router does not, and a router that has just forgotten its set
holds nothing.

Discarding on the way out rather than on the way back in means a cluster that becomes able to carry the
feature again starts from nothing, rather than from entries describing a state that moved on meanwhile.

### The gap that remains

Two controllers can disagree only by one of them holding a *stale supported* answer: a member it observed as
capable, which was then downgraded and restarted, while that controller never reconnects to it. A missing
answer cannot cause it, since an unseen member narrows the mask to nothing.

So it takes a downgrade plus a partition sustained across the whole window, and it resolves as soon as the
partition heals. Bounded rather than eliminated, deliberately.

### Faults from the far end

A fault may be reported by either endpoint, and the reporter decides how to send it from its own
capability check. So a gossip-mode router can tombstone a link it did not dial, under its own
ownership (`gossipClient.NotifyLinkFault` keys the tombstone to the link but owns it as the
reporter). With legacy reports out of gossip there is no live entry under the dialer for that
tombstone to fight, so the removal simply lands. Under the translating design the same fault
produced a live entry under the dialer and a tombstone under the reporter, and the two would flap.

Whether a router should fault-tombstone a link it did not dial at all, rather than report it as
its own, is worth settling separately; the reports and connection-state paths already scope
themselves to dialed links and this one does not.

### Keeping the legacy path unchanged

The reason routers that do not gossip are safe on this path is that they get the path 2.0 proved, and that
argument only holds while the code has not drifted. So the pre-gossip report and fault handling stays close
to how it stood in 2.0.

Close, not frozen. The goal of this work is links that behave correctly, and moving them to gossip is a
means to that rather than the end, so a real defect on the legacy path gets fixed there too. What fidelity
rules out is *unexamined* drift: a change nobody decided to make, which then cannot be told apart from a
version incompatibility.

So where gossip-links improved the legacy path, the improvement stays and is tracked for backport to 2.0 and
1.6 rather than undone. Those improvements are why this path is not byte-identical to 2.0:

- `LinkManager.RouterReportedLink` refuses a report from a router that is not the link's source. Without it
  a report naming an existing link id at a higher iteration replaces the link and takes over as its source.
- `faultHandler.handleFaultedLink` ignores a fault from a router that is not an endpoint of the link.
- The fault forward to the peer is bounded. It runs on a goroutine spawned per fault message, so an
  unbounded send parks one goroutine per fault forwarded to a stalled router.
- The full-refresh prune resolves the reporting router's links through the per-router index rather than
  scanning the whole link table.

### What landed

The steps below are done. Recorded because several ended up differing from the plan in ways worth knowing.

**Legacy reports stay out of gossip.** `routerLinkHandler.HandleLinks` is back to its pre-gossip body,
`linksToPrune` with it. The `IsLeader` gate, `handleLinksAsLeader`, `handleLinksAsNonLeader`,
`NotifyLinkViaGossip` and `ReconcileLinksViaGossip` are gone.

**Fault removal picks the mechanism that published the link.** `faultHandler.handleFaultedLink` branches on
`Network.IsLinkInGossip`, which asks whether a live entry exists for the link under its dialer. The branch is
on state, not on who reported the fault: a router in legacy mode can report a fault for a link it accepted
but did not dial, and that link may be gossip-managed by its dialer.

**Availability is per capability, decided by controllers, told to routers.** This replaced the original plan,
which had the router detect its own mode flip and retract its entries with tombstones. That plan rested on
the router's per-connection view, which is the thing that is wrong in the case that matters. It also could
not have survived a second gossip feature: a store-wide answer would switch link gossip off during the
upgrade that added terminator gossip.

**The reject message was not built.** The plan had controllers answering a refused delta with an explicit
"unavailable", and telling a legacy-reporting router to switch back. Neither is needed once the cluster mask
is reported periodically: a router learns it must stop before it is ever refused, and learns it may start
again on the next status message. What refusal remains is a backstop for the gap between a controller
deciding and a router hearing.

### Still to do

- Make `validateGossip` version-aware in the links test. A 1.6 router has no local gossip store, so
  `inLocalGossip` is meaningless for it, and the check has to be scoped to routers that gossip.
- Build the mixed-version links-test model described above.
- An axiom for the failure this removed: two mechanisms describing one entity while sharing a keyspace and a
  version space. The rule is that exactly one owns it, and ownership is derivable from state rather than
  from who reported it, which is also what makes the fault branch correct.

## Files

| Area | Files |
|---|---|
| Proto | `common/pb/gossip_pb/gossip.proto`, `common/pb/ctrl_pb/ctrl.proto` |
| Router gossip | `router/gossip.go`, `router/canary.go` |
| Controller gossip store | `controller/gossip/store.go`, `controller/gossip/state_type.go`, `controller/gossip/handlers.go`, `controller/gossip/anti_entropy.go`, `controller/gossip/lifecycle.go` |
| Link gossip | `controller/network/link_gossip.go` |
| Canary gossip | `controller/network/canary_gossip.go` |
| Controller handlers | `controller/handler_ctrl/canary.go`, `controller/handler_ctrl/gossip.go` |
| Link registry | `router/link/link_registry.go`, `router/link/link_events.go` |
