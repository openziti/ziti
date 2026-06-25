# Gossip State: Patterns, Pitfalls, and Lessons Learned

This captures the recurring problems we hit building the link-state gossip
subsystem (controller link store, router gossip client, canary/anti-entropy),
so the next subsystem to move onto gossip (terminators) can avoid re-learning
them. Most of these are not link-specific; they are general to any
eventually-consistent, owner-replicated state that flows router -> controller
and controller <-> controller with tombstones.

If you read nothing else, read this: **almost every bug was a variant of "a
removal was lost or arrived out of order relative to a live update, and nothing
reconciled the result back to the source of truth."** Design for that up front.

## The architecture in one paragraph

Each item of state (a link, soon a terminator) has an **owner** (the router
that created it). The owner advertises live entries via gossip; removal is a
**tombstone** (a versioned "this is gone" marker) rather than a delete, so the
removal can propagate. Entries are versioned for ordering. Controllers replicate
among themselves (anti-entropy) and derive a **manager view** (the link manager
/ terminator registry) that drives real behavior (circuit pathing, dialing).
A **canary** periodically carries each owner's entry count/hash so a controller
can detect when its view has drifted and trigger a digest (catch-up) exchange.

The layers, each of which can independently diverge:

```
source of truth (router link/terminator registry)
  -> router's own gossip set (what it advertises)
    -> controller gossip store (replicated, versioned, tombstoned)
      -> controller manager view (drives pathing/dialing)
```

## Pitfalls

### 1. Live-vs-tombstone ordering — the bug that keeps coming back

The single most common failure. Anywhere a "live" update and a "remove"
(tombstone) for the same key can be produced or delivered concurrently, a stale
live write can land after the tombstone and resurrect the entry. We hit this at
**every layer**:

- **Controller store listener** (#7): the store applied entries in version
  order under a lock, but delivered listener callbacks *outside* the lock, so a
  stale `entryChanged` could fire after a newer `entryRemoved` and leave the
  manager holding a tombstoned link.
- **Router's own advertised set** (#8): `NotifyLinkFault` (event loop) removed a
  key while `NotifyLinks` (rate-limiter pool) re-added it live ~20us later with a
  higher version; last-writer-wins, no version check -> orphaned live entry.
- **Canary** (#9, see #4): the detection channel itself got resurrected/stale.

**Lessons:**
- Make every gossip-state container **version-ordered**: a write applies only if
  its version is strictly newer than what's present, and tombstones are entries
  too (not deletes), so a stale live write loses the version comparison.
- If a container can't be fully version-ordered cheaply, **reconcile it against
  the source of truth on a timer** (see #5) so resurrection self-heals.
- Watch for state that is "version-ordered on apply but not on delivery" — the
  ordering guarantee must extend to whatever consumes the callbacks.

### 2. Tombstone ownership must match the live entry's owner

A tombstone only supersedes a live entry if it lands in the **same owner
namespace**. The acceptor side of a link reporting a fault produces a tombstone
owned by the *acceptor*, which does **not** supersede the dialer-owned live
entry. The authoritative removal is the **owner's own** tombstone.

This is why the dialer must be told to close (so it emits its own tombstone),
and why a fault from one side has to be forwarded to / re-emitted by the owner
rather than assumed to clean up the owner's entry.

**Lesson:** define the owner of each entry unambiguously, and make sure removals
are emitted by (or routed to) that owner. Cross-actor "I think this is dead"
signals must turn into the owner's tombstone, not a foreign-owned one.

### 3. Stamp versions at event time, not send time

If a version is assigned when a delta is *sent* rather than when the state
*changed*, a live entry that was queued-then-sent-late can outrank a tombstone
created in between. Assign the version at the moment the event happens (e.g. at
collection in the event loop), so ordering reflects causality, not scheduling.

### 4. The detection channel (canary) is gossip state too — and can go stale

The canary is what lets a controller notice its view drifted. But the canary is
*itself* replicated gossip, so it can be stale or missed, and then it
**masks** the very divergence it's supposed to catch: a controller with a stale
canary that agrees with its stale store sees no mismatch and never reconciles
(#9).

Two compounding mistakes we made:
- The canary was owned by the *receiving controller* and versioned by that
  controller's clock, so a router whose subscription controller changed over
  time produced entries under multiple owners; a blind last-writer-wins listener
  could install an older one.
- The canary had no version-ordered apply and no self-correction.

**Fix that worked:** own the canary by the **router** (single source) and
version it by the **router's own monotonic sequence**, so every controller
orders it in the same space and a stale copy is rejected by version. See #7's
principle applied to the detection channel.

**Lesson:** the staleness-detection signal needs the *same* ordering discipline
and single-source-of-truth ownership as the data it guards.

### 5. Reconcile against the source of truth — don't rely on perfect mutation

The most robust correctness mechanism is not flawless concurrent maintenance of
the gossip set; it is periodically **re-deriving** it from the authoritative
source and tombstoning anything that doesn't belong. The router knows its
established links/terminators; the controller knows what each connected owner
claims. A cheap periodic sweep that removes entries with no backing source
object converges away *any* divergence regardless of how it arose (races,
dropped deltas, anti-entropy gaps).

We kept trying to make individual mutation paths perfect (and kept finding new
races and TOCTOU windows). Maintain-by-reconcile is simpler to get right than
maintain-by-perfect-mutation, and it tolerates temporary staleness in exchange
for guaranteed convergence — usually the right trade for this kind of state.

**Lesson:** for each derived layer, add a reconcile sweep against its upstream
source of truth. Make the interval configurable; it can be generous (tens of
seconds) since the cost of transient staleness is low (see #9-impact note).

### 6. Tombstone TTL must exceed the worst-case partition-plus-reconcile window

Tombstones are reaped after a TTL so the store doesn't grow forever. But if a
node misses a tombstone while partitioned and the tombstone is then reaped
everywhere else before that node reconnects, the node is left with a live entry
that **nothing can correct** — there is no surviving tombstone, and "absence" on
peers is indistinguishable from "never had it" (#9, the controller variant).

**Lessons:**
- Set TTL > realistic worst-case partition duration, OR
- make reconciliation **owner-authoritative**: the owner's current set is the
  truth, so an entry absent from the owner's advertised set is removed elsewhere
  without needing a surviving tombstone (this is just #5 applied cross-node), OR
- ensure every node reconciles directly with the owner, not only via peer
  replication of (reapable) tombstones.

### 7. Best-effort sends plus coalescing silently drop removals

A fault that failed to send (controller unavailable during chaos) and was then
**coalesced** into a newer iteration's fault meant only the newer iteration was
ever tombstoned; the older iteration's entry lived forever (#8, original cause).
Any place where pending removals are merged/deduped by key can drop a removal
that still needed to be delivered for a *different* version/iteration.

**Lesson:** make removals durable — retried until acknowledged, or backstopped
by the reconcile sweep (#5). Don't let coalescing collapse removals for distinct
versions of the same key.

### 8. Heartbeat/liveness bounds transport death, not logical retirement

Link heartbeats detect a dead connection within ~60s, so a genuinely dead link
is cleaned up on both ends quickly. But heartbeats say nothing about a link that
is transport-healthy yet **logically retired** (e.g. lost a duplicate-link
dedup, or a policy decision). Those need explicit propagation; you cannot lean
on liveness timeouts to bound logical state.

**Lesson:** don't assume "it'll time out eventually." Logical removals need an
explicit, reconciled path. (Terminators have an analog: an SDK that is connected
but whose terminator was administratively/duplicate-removed.)

### 9. Observe every layer, and validate *across* layers

We were blind for a long time because we could inspect the controller store but
not the router's gossip set or the canary, so we *inferred* where divergence
was instead of seeing it. Adding an inspect for each layer
(`gossip-links`, `gossip-store`, `gossip-canaries`) and a cross-layer
`validate gossip` (source registry vs local gossip vs controller gossip vs
manager) was what turned "something is wrong somewhere" into "this exact entry
on this node, in this layer." The cross-layer validator also became the test's
correctness gate — link counts alone passed while gossip was quietly diverging.

**Lessons (do these *first* for terminators):**
- Add an inspect for **every** layer of the chain, including the detection
  channel. Don't ship a layer you can't dump.
- Build a cross-layer validator that pins divergence to a specific entry +
  layer, with per-entry presence flags (`inSource` / `inLocalGossip` /
  `inCtrlGossip`). Wire it into the chaos test's validation phase so a divergence
  fails the run instead of self-healing before anyone looks.
- Distinguish error directions in the output (X has it / Y doesn't) — the
  direction tells you which layer and which fix.

### 10. Operational: retention budgets scale with components-per-host

Not a gossip bug, but it killed a soak: size-rotated component logs defaulted to
a per-component budget (50MB x 10) that was fine for a one-component controller
host but 5.5G on a host packing 10 routers, which filled the disk and took those
routers down — surfacing as "missing routers," not "disk full." When many
instances share a host, retention/quotas must be sized per *host*, not per
*component*.

## Impact note: what a stale entry actually costs

Worth knowing when deciding how hard to fight transient staleness. A stale entry
in a **router's own** advertised set (present locally, absent on the controller)
is low-harm: it does not affect circuit pathing or dialing (the controller never
holds it) and does not affect the data plane (forwarding uses the registry, not
the gossip set). Its only cost is gossip churn — a canary count/hash mismatch
that triggers repeated digest exchanges until reconciled. A stale entry in the
**controller's manager view** (a phantom live link) is the harmful one: it can
make the controller path a circuit over a dead link. Prioritize correctness of
the controller-derived view; tolerate brief router-local staleness.

## Conversion step 1: make the router gossip client generic

There is an asymmetry today worth fixing before terminators start. The
**controller** gossip side is already generic — `gossip.Store` holds many
`StateType[T]` registered via `Register[T]` (links and canaries are two
instances). The **router** side (`router/gossip.go`, `gossipClient`) is hardcoded
to links: the store type, the source-of-truth iterator (`linkIterator` +
`IsDialed`), the key (`linkGossipKey`), the value marshaling (`marshalLink`), and
the public API (`NotifyLinks`/`NotifyLinkFault`) are all link-specific. A
terminator subsystem would otherwise duplicate the whole client (clock,
currentEntries, canary, digest, anti-entropy, tombstones, reconcile).

The store is router-local and ephemeral, so this refactor has no wire/version/API
surface to freeze — it's cheap and safe to do, and cheap to tweak later.

**It's one client hosting multiple store types, not one client per type.** The
wire format already assumes this: `CanaryGossipValue` carries `max_sent_versions`,
`entry_hashes`, and `entry_counts` as `map<string, ...>` **keyed by store type**,
and the router's `GetEntryCounts()` already returns a one-element map keyed by
store type. The design was always "one transport/clock/canary core, N state
types," mirroring the controller; we just only ever populated "links."

**The seam** — a per-store-type source adapter supplying exactly the link
coupling points:

```go
type gossipSource interface {
    StoreType() string
    // IterateAdvertised yields the (key, marshaled value) for each
    // source-of-truth item that should be advertised. Drives reconcileEntries,
    // HandleDigest, and the canary hash/count.
    IterateAdvertised(fn func(key string, value []byte))
}
```

- links adapter: iterate `linkIterator`, filter `IsDialed()`, key
  `linkId:iteration`, marshal `RouterLinks_RouterLink`.
- terminators adapter: iterate established terminators in `xgress_edge` /
  `xgress_edge_tunnel` → key/value.

**What changes vs stays:**
- *Per store type:* `currentEntries`, the `maxSent`/`entry_hashes`/`entry_counts`
  entries, and the registered `gossipSource`. (Namespace currentEntries by store
  type, or hold a map of per-type sub-stores.)
- *Shared core:* the Lamport clock, `sendDelta`, the digest exchange,
  `applyTombstones`/`newTombstone`/`buildSupersededTombstones`,
  `sendToStaleControllers`, and the canary emission loop.
- *Generic methods over the source:* `reconcileEntries`, `HandleDigest`, the
  canary count/hash.
- *Public API:* `NotifyLinks`/`NotifyLinkFault` become a thin link adapter over
  generic `NotifyEntries([]versionedEntry)` / `NotifyTombstone(key)`.

This is the router mirror of the controller's `Register[T]`. Do it as a
**behavior-preserving** first pass (links must behave identically; build/vet and
the `router/link` tests as guards), against a known-good baseline (after the link
soak is clean, so a refactor regression can't be confused with a residual gossip
bug). Expect to tweak the seam when the terminator adapter reveals what links
didn't need (terminators have no "dialed" concept and different identity/keying).

## Checklist for the terminators conversion

- [ ] Extract a generic router gossip core hosting multiple store types (mirror
      the controller's `StateType[T]`); write a terminator source adapter
      (see "Conversion step 1" above). Behavior-preserving first pass.

- [ ] Define the owner of a terminator entry and ensure all removals are
      owner-emitted/owner-routed (#2).
- [ ] Version-order the store and treat tombstones as versioned entries; extend
      ordering to listener delivery (#1, #3).
- [ ] Add a reconcile sweep from each derived layer to its source of truth (#5),
      configurable interval.
- [ ] Make the detection/canary channel single-source-owned and version-ordered
      (#4).
- [ ] Choose a tombstone TTL > worst-case partition, or make reconciliation
      owner-authoritative (#6).
- [ ] Don't coalesce removals across distinct versions; back them with the sweep
      (#7).
- [ ] Add per-layer inspects and a cross-layer validator wired into chaos
      validation, *before* chasing bugs (#9).
