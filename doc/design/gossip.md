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

Controllers periodically exchange digests, lists of `(key, version)` pairs, to
repair missed broadcasts. The sender picks a rotating peer every 30 seconds,
sends its full digest, and the peer responds with any entries the sender is
missing or has stale.

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
