# Link Latency and Metrics over Gossip

## The problem

Routers report metrics to every controller on a fixed interval. At fabric scale
the bulk of that traffic is per-link metric series (`link.<id>.latency`,
`link.<id>.tx.bytesrate`, the `dropped_*` meters, and so on), and there are a lot
of them: roughly five thousand series per router times hundreds of routers, sent
to every controller, every interval. Every controller unmarshals and processes
the whole message, even though almost none of it drives a control-plane decision.

This is about that per-controller ingestion cost and the fan-out, not the
controller OOM seen when reading metrics fabric-wide. That OOM came from
materializing every router's metric registry on the read path, so it is a
function of how many series each router holds, addressed separately by fixing the
metric leak. Narrowing where the firehose is sent does not change what a
fabric-wide read pulls from the routers.

Almost none of that is needed by more than one controller. The only thing the
control plane consumes from router metrics for its own decisions is per-link
latency, and it needs that on every controller because every controller routes.
Everything else is observability: useful, but it only has to land somewhere, not
everywhere.

So the plan is to split the two concerns. Latency, which every controller needs,
moves into gossip, which already replicates router state efficiently and is
processed once per controller regardless of how many routers report. The rest of
the metrics, which only needs to land once, goes to a single controller instead
of all of them.

## What the control plane actually needs

`network.AcceptMetricsMsg` is the only routing consumer of router metrics. Per
link it reads the `latency` mean plus the `queue_time` mean, sums them into a
latency cost, and sets the link's source or destination latency depending on
which router reported. Smart routing uses that cost.

Nothing else in the metrics message feeds a control-plane decision. The tx/rx
rates, message sizes, and dropped-message meters are observability only, and the
controller's metric filter already discards them by default. Per-router metrics
(`host.`, `process.`, `gossip.`, `pool.`, `ctrl.`) are small and unrelated to
links; they are out of scope here.

The whole scope of "metrics in gossip" is therefore latency, with `queue_time`
folded into the same value the router publishes.

## Design: latency over gossip

### A separate link-metrics store

Latency rides a new gossip store type, `link-metrics`, distinct from the
link-state store. The router gossip client is already generic over store types,
so this is an additive store, not new machinery.

The entry is keyed by link id and owned by the reporting router. The value is the
latency cost (nanoseconds) together with the link's current dial iteration (see
Iteration and lifetime below). Both ends of a link publish their own measurement
of it, exactly as both ends report latency through metrics today; the controller
listener sets source or destination latency based on which router owns the entry,
the same check `AcceptMetricsMsg` already makes. Directionality comes for free,
with no special casing.

Keeping latency in its own store, rather than as a field on the link-state entry,
matters. Latency is volatile: it moves on every heartbeat. Link state
(destination router, connection state) is stable. Folding latency into the
link-state entry would bump that entry's version on every update, churning the
link-state store's hash and count and keeping the staleness and reconcile paths
perpetually busy, the opposite of the convergence we just stabilized. A separate
store confines the churn to where the volatility actually is.

### Iteration and lifetime

Link ids are stable, but a link's dial iteration advances on every re-dial, and
the controller builds a fresh link object with default latency for each new
iteration. Keying latency by link id alone is therefore not enough on its own: if
the measured latency after a re-dial is close to what it was before, it misses the
change threshold and never republishes, leaving the new iteration's link at
default latency or reusing stale lifetime data.

Two rules close that gap. First, the value carries the iteration alongside the
latency cost, and the router force-publishes the entry whenever the iteration
advances, regardless of the change threshold, so every new iteration pushes a
fresh latency promptly instead of waiting for a meaningful change.

Second, the controller applies a latency only when the entry's iteration equals
the link's current iteration. An older iteration is ignored. A newer iteration is
left unapplied rather than written onto the current link: because link state and
link metrics are separate stores that replicate independently, a metrics entry for
iteration N+1 can arrive before the link-state entry that creates the N+1 link,
and applying it to the still-current N link would burn it (the link manager then
replaces the link with default latency when N+1 arrives, and the metrics entry,
already delivered, does not fire again).

To cover that ordering, link creation reconciles from the store, and it does so
for both ends of the link. Link state is owned only by the source (dialer), but
link-metrics has a separate entry per reporting router, so the source and the
destination each publish one for the link. When the link-state listener creates or
replaces a link, it looks up the current link-metrics entry for that link id under
both the source router and the link's destination router, and applies each side,
source latency and destination latency, when that entry's iteration matches the
link. So whichever store's entry arrives first, and from whichever end, the
latency lands on the right link: an early metrics entry from either end waits in
the store until link creation reconciles it, and a late one is applied by the
listener once the iterations match.

Keying by link id rather than `linkId:iteration` keeps one entry per link across
re-dials and avoids a tombstone per iteration. The entry is created when the link
first comes up, refreshed by the force-publish on each iteration change, and
tombstoned when the link is removed, with owner-level cleanup handled by the
generalized owner-drop and epoch paths above.

### Publishing cadence

Routing only needs latency fresh to about `cycleSeconds` (15s by default), not to
the heartbeat. So the router publishes a link's latency to gossip only when it has
changed meaningfully since the last publish (for example, the mean moved more than
some percentage or some absolute amount) and at most once per interval per link. A
link whose latency is steady rarely publishes. This keeps gossip deltas
proportional to actual latency variation, well under the firehose that ships
everything every interval. The one exception is a dial-iteration change, which
always force-publishes (see Iteration and lifetime), so a re-dialed link gets
fresh latency immediately rather than waiting for the threshold.

### Router side

The heartbeat callback already measures link latency. A periodic publisher reads
each link's mean and writes it to the `link-metrics` store under the cadence rule
above.

### Controller side

A listener on the `link-metrics` store sets source or destination latency, taking
over from the latency block in `AcceptMetricsMsg`. It applies an entry only when
the entry's iteration equals the link's current iteration; older iterations are
dropped and newer ones are left for link creation to reconcile from the store (see
Iteration and lifetime).

During the transition both code paths exist, so precedence has to be explicit
rather than "gossip wins when present." `AcceptMetricsMsg` keeps setting latency
from metrics for as long as it is registered, and because the per-link latency
and queue_time histograms still travel to the subscription controller for
observability, that path would otherwise overwrite the gossip-derived value on
every metrics message. The router resolves it by signaling its mode on the
message: a new `linkLatencyInGossip` flag on `MetricsMessage`, set exactly when
the router is publishing link latency over gossip (that is, when
`AllControllersHaveCapability(ControllerLinkGossip)` and the gossip publisher is
active). `AcceptMetricsMsg` skips its latency-extraction block when the flag is
set. The histograms still travel in the message and still produce observability
events; they just stop feeding routing.

The flag is per message, so a mixed fleet is handled per router: a flagged
router's latency comes only from gossip, an unflagged (older) router's only from
metrics, and neither double-writes a link. Because the router gossips all of its
link latencies or none (gated on the one capability), a single bool is the right
granularity, and the consumer check is one test rather than a per-link lookup
against the gossip store. It does require adding the field to the `MetricsMessage`
proto in openziti/metrics, next to the existing `doNotPropagate`; that ships with
the gossip release. Once every router publishes latency over gossip, the latency
block in `AcceptMetricsMsg` is removed and the flag is no longer read.

## Store lifecycle and anti-entropy

A new store type is not just a value plus a listener. The link-state store carries
a lifecycle that link-metrics has to match, or its entries leak and diverge the
same way links would have.

Registration is the easy part: link-metrics registers like the others, with
`gossip.Register[...]` plus `RegisterGossipType("link-metrics", ...)`, on the
controller, and as another store in the router's generic gossip client. Once it
is a store in the client, it is automatically included in the router's canary
payload (`GetEntryHashes`/`GetEntryCounts` iterate every store), and the
controller's staleness check is already per-store-type (it iterates the canary's
per-type hashes and counts and looks each up with `GetGossipType`). So the
steady-state reconcile and the staleness-driven digest re-trigger cover
link-metrics for free once it is registered on both sides.

What does not come for free are the three places that are still hardcoded to the
links store:

- Connect-time digest. On router bind the controller proactively pushes a links
  digest so the router can reconcile on reconnect (`handler_ctrl/bind.go`). The
  router-initiated digest request already carries per-type hashes and so covers
  link-metrics on reconnect, but the proactive push should be widened to every
  registered store type for parity.
- Owner drop on router delete. `Network.HandleRouterDelete` calls
  `LinkGossipType.DropOwner` so a removed router's entries are tombstoned and
  compacted out (`network.go`). link-metrics needs the same drop, or a deleted
  router's latency entries accumulate.
- Epoch cleanup on restart. When the canary listener sees a router's epoch
  advance (a restart) it calls `LinkGossipType.DeleteByOwnerBefore`
  (`canary_gossip.go`) to clear old-lifetime entries. link-metrics needs the same,
  or stale-epoch latency lingers after a restart.

Rather than add a link-metrics line at each of these three sites, they should
iterate every registered gossip store type, so this store and any future one (the
terminators store being the obvious next) are covered without another round of
hardcoded edits. That means widening the `StateTypeInfo` interface to expose
`DropOwner` and `DeleteByOwnerBefore`, which today live only on the concrete
`StateType[T]`, and driving the connect-time digest from the registered store-type
names. This generalization is the bulk of the lifecycle work; the latency value
and listener are small by comparison.

## Metrics to a single controller

With latency in gossip, the metrics message is no longer routing-critical, so it
no longer has to reach every controller. It goes to a single controller instead,
the subscription controller (the same one that already receives canaries and link
reports), where it is converted to events.

Once the message goes to a single controller, every event path on that controller
(the configured filtered subscriptions, the usage adapter, and the dispatcher's
unfiltered relay) runs there and nowhere else, simply because no other controller
receives the message. Single-controller event emission therefore falls out of the
narrowing itself, with no change to the event-propagation logic.

A couple of accuracy notes, since `DoNotPropagate` is easy to over-read. It is
purely a cross-controller dedup: the reporter sets it on all but the first
controller it sends to so that a given event is emitted by only one of them, and
"propagate" means emit locally, not forward to peers. With a single recipient it
is always the first, so it is vestigial for metrics once the firehose is narrowed,
and we leave it alone. Also, today the filtered subscriptions and the usage
adapter honor it while the unfiltered relay does not, so the relay currently emits
on every controller that receives the message; narrowing just brings it in line
with the others rather than changing intended behavior. Per-link observability
metrics flow with the rest of the message to that one controller.

The `linkLatencyInGossip` flag is unrelated to any of this: it gates the
routing-latency path in `AcceptMetricsMsg`, not event extraction, so the event
paths still see latency and continue to emit latency observability events.

## Backward compatibility

Metrics in gossip rides the existing `ControllerLinkGossip` capability rather than
adding a new one: a controller that supports gossip supports latency in gossip, so
it is all or nothing on that single capability. The switch is gated by
`AllControllersHaveCapability(ControllerLinkGossip)`, the same gate the link
gossip already uses throughout `router/link`.

Gossip link state and gossip link metrics ship in the same release, so there is
no version that has one without the other. The only mixed-version case to handle
is upgrading from a release that predates gossip entirely.

While any connected controller predates gossip, the router keeps the full metrics
firehose to all controllers and controllers keep deriving latency from it, so a
mixed-version cluster mid-upgrade behaves exactly as it does today. Once every
connected controller is gossip-capable, the router publishes latency over gossip
and narrows the metrics message to the single subscription controller. The
transition is automatic; no operator action is required.

## Rollout phases

1. Add the `link-metrics` store with its full lifecycle (the per-store-type
   connect-time digest, owner drop, and epoch cleanup described above), the router
   publisher (force-publishing on iteration change), and the controller listener
   that sets latency on an exact iteration match, plus the reconcile-from-store
   step (for both the source and destination owners) when the link-state listener
   creates or replaces a link. Add the
   `linkLatencyInGossip` flag and have `AcceptMetricsMsg` honor it, so the metrics
   path stops feeding routing for flagged routers while still producing
   observability events. Leave the metrics firehose routing (still to all
   controllers) unchanged this phase. Soak-validate that gossip-derived latency
   tracks the metrics-derived value across re-dials, and that latency entries are
   dropped on router delete and cleared on restart. The lifecycle generalization
   is the bulk of this phase; otherwise it is additive and low risk.
2. Gate on `AllControllersHaveCapability(ControllerLinkGossip)`: send the metrics
   message to the subscription controller only. Event emission becomes
   single-controller because only that controller receives the message; no
   propagation-logic change is needed. Validate the per-controller ingestion drop
   and that routing is unchanged.
3. Remove the latency block from `AcceptMetricsMsg` once all routers publish over
   gossip. Cleanup.

## Open items and tuning

- Publish threshold and cadence: start at most once per 15s per link, publishing
  only on a meaningful change (for example more than 15%, or more than 2ms), and
  tune from a soak.
- `queue_time` is folded into the published latency cost, matching what
  `AcceptMetricsMsg` computes, so gossip carries one value per link per end.
