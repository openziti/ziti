# Gossip-Based Ephemeral State Distribution for OpenZiti Controllers

## Problem Statement

The OpenZiti controller cluster currently stores ephemeral state (terminators, links) in bbolt, replicated via Raft. This is architecturally mismatched:

- **Terminators** have high write volume. Each terminator has a single writer (the responsible router or SDK), so contention is not a concern, yet every update pays the cost of Raft consensus and durable storage.
- **Links** are reported by every dialing router to every controller independently, creating redundant traffic that scales as `routers * controllers`.
- Both terminators and links are **soft state** — ephemeral and reconstructible. If lost, routers can simply re-report them. The strong consistency and durability guarantees of Raft are unnecessary overhead for this class of data.

## Design Goals

1. Move ephemeral state out of bbolt/Raft into in-memory maps with gossip-based distribution.
2. Minimize stale data during network partitions.
3. Reduce redundant traffic (particularly for link reporting).
4. Leverage the fact that routers maintain connections to all (or as many as possible) controllers.

## Architecture Overview

The system introduces two mechanisms:

- **Gossip layer** for controller-to-controller distribution of ephemeral state.
- **Canary protocol** for end-to-end consistency monitoring, using routers as partition observers.

These sit alongside the existing Raft-based durable store:

| Layer | Data | Consistency | Persistence |
|---|---|---|---|
| Raft + bbolt | Data model (policies, configs, identities) | Strong (linearizable) | Durable |
| Gossip + canary | Ephemeral state (terminators, links) | Eventual (ms in practice) | In-memory only |
| Data model subscriptions | Streaming updates to routers | Index-based consistency | N/A (derived) |

## Mesh Topology and Gossip Routing

### Leader Affinity for Writes

Routers already favor sending data model updates to the Raft leader, since those commands must be applied there anyway. Sending to a follower just adds a forwarding hop. The same logic applies to ephemeral state: if the leader is the natural dissemination hub (connected to all peers for Raft replication), routing gossip origination through the leader minimizes hops. Routers can send terminator and link updates to the leader preferentially, with the leader broadcasting deltas to all followers directly.

When the leader is unavailable, routers fall back to sending updates to any connected controller, which forwards as needed.

### Full Mesh Aspiration

Raft itself does not require a full mesh — only leader-to-follower connections are strictly necessary. However, for gossip efficiency, a full controller mesh is desirable. Controllers can maintain peer-to-peer connections independently of what Raft requires, using the existing `ctrl.mesh` channel infrastructure. A full mesh allows any controller to disseminate gossip directly to all peers without relying on the leader as a relay, improving latency and removing the leader as a single point of throughput for ephemeral state.

### Handling Partial Mesh with Transparent Forwarding

In some deployments, a full mesh may not be achievable — controllers may be spread across network boundaries where not every pair can connect directly. The gossip layer must handle this gracefully with **transparent forwarding**:

- When a controller receives a gossip delta and knows of peers it cannot reach directly, it forwards the delta through a connected peer that can reach them.
- This forwarding is handled entirely within the mesh transport layer. It is **opaque to Raft** — Raft continues to see its leader-to-follower connections and is unaware that the underlying mesh is relaying messages on its behalf.
- Forwarding decisions can be based on the mesh's knowledge of peer connectivity (which peers each controller has active channels to), effectively building a routing overlay within the controller cluster.

This means the gossip layer (and the mesh layer beneath it) needs to handle three topology scenarios:

| Topology | Gossip Strategy |
|---|---|
| Full mesh | Any controller broadcasts directly to all peers |
| Star (leader-connected only) | Leader disseminates; followers forward through leader |
| Partial mesh | Controllers forward through reachable peers; mesh handles relay transparently |

In all cases, the version-based merge makes duplicate or out-of-order delivery safe. A delta arriving via two different paths is simply a no-op on the second application.

## Why Not In-Memory Raft?

Raft with an in-memory FSM (no bbolt backing) was considered and rejected:

- **Leader bottleneck**: All writes must go through the Raft leader, adding latency and creating a single point of throughput.
- **Log replication overhead**: Every terminator update becomes a log entry replicated to a majority before acknowledgment.
- **Snapshotting complexity**: Even an in-memory FSM requires snapshot/restore logic for membership changes and leader elections.
- **Unnecessary guarantees**: Raft provides linearizable consensus. Ephemeral, reconstructible data does not need this. Eventual consistency with partition-aware self-healing is sufficient.

## Data Model Subscriptions

Routers connect to as many controllers as possible but subscribe to a **single controller** for data model updates at any given time. This subscription is lease-based and rotates periodically.

- The subscription controller streams data model changes (from the durable Raft store) to the router.
- All other connected controllers include their current **data model index** in heartbeats to the router.
- The router always knows whether its subscription controller is behind.

## The Canary Protocol

### Mechanism

The canary protocol provides end-to-end observability of gossip health by using routers as distributed partition detectors.

**Router behavior:**
- Router R picks controller C1 as its subscription controller (lease-based, with jitter).
- R sends a monotonically increasing **canary sequence number** to C1 on each tick (e.g., every 5-10 seconds).
- R receives heartbeats from all connected controllers. Each heartbeat contains:
  - That controller's view of R's latest canary sequence number.
  - That controller's current data model index.

**Controller behavior:**
- C1 receives R's canary and gossips it to C2, C3, etc.
- Every controller includes in its heartbeat to R: `(canary_for_R, data_model_index)`.
- Controllers monitor incoming gossip to detect stale canaries from peers.

### Partition Detection

The router assembles a freshness matrix from the heartbeats it receives:

| Scenario | C1 (subscribed) | C2, C3 | Interpretation |
|---|---|---|---|
| Healthy | canary=current, dmi=current | canary=current, dmi=current | All good |
| C1 partitioned from cluster | canary=current, dmi=**stale** | canary=**stale**, dmi=current | C1 got canary directly but can't reach Raft leader or gossip peers |
| C1 slow gossip | canary=current, dmi=current | canary=**stale**, dmi=current | C1 is reachable and up to date, but its gossip to peers is lagging |
| R-to-C1 link degraded | canary=**stale**, dmi=stale | canary=**stale**, dmi=current | C1 isn't receiving R's canaries at all |

The canary gives routers something the data model index alone cannot: proof that the **gossip path** is working end-to-end. The data model index only proves Raft replication is functioning. For ephemeral state (terminators, links), gossip health is what matters.

### Canary-Per-Controller Variant

Optionally, routers can send canaries to **all** connected controllers rather than just the subscription controller. This gives every controller an independent proof of reachability from that router, making the freshness matrix richer. The overhead is trivial (one sequence number per heartbeat tick per connection).

## Controller-Side Proactive Failover

Rather than relying solely on routers to switch subscriptions, controllers can self-heal:

1. **C2 monitors incoming gossip.** It knows (via gossip or inference) that R is subscribed to C1. It observes that R's canary hasn't updated in N cycles.

2. **C2 concludes C1 is partitioned** (at least for gossip). C2 doesn't know whether R-to-C1 is healthy, but it knows C1-to-C2 gossip is broken.

3. **C2 starts proactively streaming data model updates to R** on their existing control channel. R doesn't have to ask.

4. **R deduplicates naturally.** R tracks its applied data model index. If C2 sends an update R already has (because C1 is actually fine), R ignores it. The cost of a false positive is minor — just some redundant traffic.

5. **C2 backs off when gossip heals.** Once C2 starts seeing fresh canaries for R again, it knows gossip from C1 is flowing and stops the redundant stream.

The key property: **the cost of a false positive is just redundant updates**, not incorrect state. This means the detection threshold can be aggressive (e.g., 2-3 missed canaries) without risk.

## Ephemeral State Distribution

### Terminators

- Router sends terminator creates/updates/deletes to its **subscription controller** only.
- The subscription controller stores the update in its local in-memory map and gossips the delta to peers.
- Each terminator entry is versioned: `(terminator_id, value, version)`. The owning router is the single writer, so "last writer wins" is effectively "only writer wins" — no conflicts.
- **Deletes**: Use versioned tombstones. When a router disconnects, the controller that detects the disconnect increments the version and marks all of that router's terminators as dead. Tombstones are reaped after a configurable TTL (e.g., 5 minutes).
- **Fallback**: If the canary protocol detects stale gossip, the router can send terminator updates to additional controllers directly. Or controllers that detect stale canaries for a router can request a full terminator resend from that router.

### Links

- Dialing router sends link info to its **subscription controller** only (not all controllers).
- The subscription controller gossips to peers.
- Same versioned-entry model and canary-based staleness detection as terminators.
- This reduces link reporting traffic from `O(routers * controllers)` to `O(routers)`, with gossip handling distribution.

### Disconnect Handling

When a controller detects a router disconnect:
1. It broadcasts tombstones for all of that router's terminators and links.
2. When the router reconnects (possibly to a different controller), it re-reports all its state, which naturally overwrites the tombstones with fresh entries.

## Gossip Protocol Details

### Delta Propagation

On each write (terminator or link update), the receiving controller writes to its local in-memory map and propagates the delta `(key, value, version)` to peers. The propagation path depends on mesh topology:

- **Full mesh**: The receiving controller broadcasts directly to all peers.
- **Leader-connected only**: If the receiver is the leader, it broadcasts to all followers. If the receiver is a follower, it forwards to the leader, which then broadcasts.
- **Partial mesh**: The receiving controller sends to all directly connected peers. Peers that are not directly reachable receive the delta via transparent forwarding through the mesh relay layer.

In all cases, version-based merge ensures duplicate delivery is harmless.

### Anti-Entropy

Periodically (e.g., every 10-30 seconds), controllers exchange **digests** — a list of `(key, version)` pairs. If a peer has a newer version for any key, the stale controller requests the full value. Anti-entropy serves as the consistency backstop, healing any updates missed during transient gossip failures.

### Confirmed Propagation

Most ephemeral state updates are fire-and-forget — brief staleness is tolerable because the consequence is a suboptimal routing decision, not a correctness failure. However, some cases require confirmation that all controllers have received an update (e.g., the first terminator for a service, or an SDK that needs to know it is routable before proceeding).

For these cases, the gossip layer supports **acknowledged dissemination**:

1. The originating controller sends the delta to peers with an `AckRequested` flag and a `RequestId`.
2. Each peer applies the delta and responds with a `GossipAckType` message containing the `RequestId`.
3. The originator collects ACKs. Once all peers (or a sufficient subset) have acknowledged, it confirms propagation to the caller.

```
Router -> C1: CreateTerminator (confirmed=true)
C1: write to local map
C1 -> peers: GossipDelta (request_id=X, ack_requested=true)
    peers -> C1: GossipAck (request_id=X)
C1 -> Router: CreateTerminatorResponse (propagated=true)
```

With 3-5 controllers on a LAN, confirmed propagation adds 1-2ms over the async path. There is no consensus overhead — no log persistence, no ordering, no leader requirement. It is a simple broadcast-and-collect-ACKs round.

**Timeout behavior**: If a peer does not ACK within a configurable window (e.g., 500ms), the originator can still respond with a partial propagation indication. The caller decides whether to retry or proceed. Anti-entropy will heal the lagging peer regardless.

### Tombstone Reaping

Tombstoned entries are garbage collected after a configurable TTL (e.g., 5 minutes). The TTL must be long enough that all controllers have received the tombstone via at least one anti-entropy cycle.

### In-Memory Data Structure

Each controller maintains:

```
map[string]*VersionedEntry

type VersionedEntry struct {
    Value     interface{}
    Version   uint64
    Owner     string    // router ID — the single writer
    Tombstone bool
    UpdatedAt time.Time
}
```

On merge: if incoming version > local version, accept. If equal, no-op. If less, ignore (stale).

## Stability Safeguards

### Staleness Window

Do not react to a single missed canary. Use a sliding window, e.g., "C1's canary for R is 3+ ticks behind." This absorbs transient network hiccups, GC pauses, and scheduling delays.

### Hysteresis on Failover

Once C2 starts proactively streaming to R, require sustained gossip health (e.g., 5 consecutive current canaries) before backing off. This prevents flapping during unstable network conditions.

### Subscription Rotation with Jitter

When routers pick or rotate their subscription controller, add jitter to the lease duration. This prevents all routers from rotating simultaneously and overloading a single controller (thundering herd). If a router detects its subscription controller is partitioned, it picks a new one with randomness rather than deterministically converging on the same "healthiest" controller.

## Relationship to the Existing Raft Mesh

### What Already Exists

Controllers already maintain a peer-to-peer mesh for Raft consensus. This mesh provides:

- **Persistent peer channels**: Each controller pair maintains a `ctrl.mesh` channel (built on `openziti/channel/v4`) with mutual TLS authentication via SPIFFE certificates and cluster ID validation. Peers are discovered via configured addresses and accepted via `AcceptUnderlay()`.
- **Multiplexed message types**: The mesh already carries multiple traffic types over a single channel per peer:
  - Raft log replication (`RaftConnectType`, `RaftDataType`, `RaftDisconnectType`)
  - Command forwarding to the leader (`NewLogEntryType`)
  - Cluster management (add/remove peer, transfer leadership)
- **Header-based metadata exchange**: Controllers already gossip API addresses via `ApiAddressesHeader` on peer connections — a rudimentary form of sideband state distribution.
- **Heartbeat infrastructure**: Bidirectional heartbeats on mesh channels with latency tracking and automatic cleanup of unresponsive connections (`CheckHeartBeat`, `CloseUnresponsiveTimeout`).
- **Cluster event system**: Events for peer connect/disconnect, leadership changes, and membership changes (`ClusterPeerConnected`, `ClusterLeadershipGained`, etc.) dispatched asynchronously to registered handlers.

### What the Gossip Layer Adds

The gossip layer does **not** require a new transport or connection management system. It is a set of new message types and handlers on the existing mesh channels:

| Existing Infrastructure | Gossip Layer Addition |
|---|---|
| `ctrl.mesh` peer channels | New message types for delta propagation and anti-entropy |
| Heartbeat messages | Extended with canary sequence numbers and data model index |
| `ApiAddressesHeader` pattern | Generalized to ephemeral state headers |
| Cluster event handlers | New handlers to trigger tombstone broadcasts on peer disconnect |
| Peer connection lifecycle | Triggers full state sync to newly connected peers |

Concretely, the new message types would be:

```go
const (
    // Gossip message types (ephemeral state)
    GossipDeltaType        = 2060  // Single (key, value, version) update
    GossipDigestType       = 2061  // Anti-entropy digest: [(key, version), ...]
    GossipDigestResponseType = 2062  // Response with full entries for stale keys
    GossipTombstoneType    = 2063  // Bulk tombstone broadcast (e.g., router disconnect)
)
```

These are registered as handlers on the existing mesh `BindHandler`, exactly like `NewLogEntryType` and the cluster management messages are today.

### Reusing the Peer Lifecycle

The existing mesh already handles the hard parts of peer management:

- **Peer connect**: When a new controller joins (or reconnects), the `PeerConnected` event fires. The gossip layer hooks this event to perform a **full state snapshot transfer** — the existing controller sends its complete in-memory map to the new peer, bringing it up to date immediately rather than waiting for anti-entropy to converge.
- **Peer disconnect**: When a peer drops, the `PeerDisconnected` event fires. The gossip layer does not need to take special action here — the disconnected controller simply stops receiving deltas, and anti-entropy will heal its state when it reconnects.
- **Read-only mode**: The existing mesh already detects version mismatches between peers and enters read-only mode. The gossip layer respects this — if the mesh is read-only (version mismatch), gossip continues operating (ephemeral state is version-independent) but the canary protocol flags the version-mismatched controller as potentially stale for data model subscriptions.

### What Changes in Existing Flows

**Terminator and link lifecycle commands** currently flow through Raft:
1. Router sends terminator create/update/delete to a controller.
2. Controller wraps it as a Raft command and applies it through the FSM.
3. FSM writes to bbolt.
4. If the receiving controller is not the leader, the command is forwarded to the leader via `NewLogEntryType`.

With the gossip layer, this changes to:
1. Router sends terminator create/update/delete to the leader (preferred) or its subscription controller.
2. Controller writes to its local in-memory map (no Raft, no bbolt).
3. Controller propagates a `GossipDeltaType` message to peers — directly if fully meshed, via the leader if star topology, or via mesh relay if partially meshed.
4. Peers update their local in-memory maps.

Routers favor sending to the leader for the same reason they already favor it for data model updates: it avoids a forwarding hop. But any controller can accept writes — if the leader is unreachable, the receiving controller forwards through whatever mesh path is available. This is a soft preference, not a hard requirement.

**Heartbeat messages** on both the mesh (controller-to-controller) and control channel (controller-to-router) are extended:
- **Controller-to-router heartbeats**: Add `CanarySeqHeader` (the controller's view of that router's canary) and `DataModelIndexHeader` (the controller's current Raft applied index). These are lightweight — two uint64 headers on messages that are already being sent.
- **Router-to-controller heartbeats**: Add `CanarySeqHeader` (the router's current canary sequence number). Again, a single uint64 on an existing message.

No new connections, no new goroutines for connection management, no new TLS handshakes. The gossip layer is purely additive message handling on infrastructure that already exists and is already maintained.

## Summary

| Concern | Approach |
|---|---|
| Terminator distribution | Single-writer gossip via subscription controller |
| Link distribution | Single-writer gossip via subscription controller |
| Partition detection | Canary protocol — routers as distributed observers |
| Data model freshness | Data model index in heartbeats + proactive failover |
| Conflict resolution | Not needed — single writer per key, version-based merge |
| Failure recovery | Routers re-report all state on reconnect; tombstones bridge the gap |
| Consistency guarantee | Eventual (milliseconds in practice), with canary-driven self-healing |
