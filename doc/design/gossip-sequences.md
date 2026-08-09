# Link Gossip Sequences

Sequence diagrams for the gossip flows described in [gossip.md](gossip.md).

## Participants

- **Router**: runs the `gossipClient`, dials links, sends gossip deltas and
  canaries
- **Subscription Controller (Sub)**: the controller a router sends gossip to
  (preferred: leader, fallback: most responsive)
- **Peer Controllers**: other controllers in the HA cluster

## 1. Router → Controllers: Link State Delta

Router establishes or faults a link, sends a gossip delta (with epoch) to the
subscription controller. The controller applies it and broadcasts to peers.

```mermaid
sequenceDiagram
    participant R as Router
    participant Sub as Sub Controller
    participant Peer as Peer Controller

    R->>Sub: GossipDelta (entries with epoch, or tombstones)
    Sub->>Sub: applyAndBroadcast (version check, notify listener)
    Sub->>Peer: GossipDelta (broadcast)
    Peer->>Peer: applyDelta (version check, notify listener)
```

`NotifyLinks()` and `NotifyLinkFault()` both flow through `sendDelta()`. If
the send fails, `NotifyLinks` leaves the link as un-notified for retry;
`NotifyLinkFault` falls back to the pending fault queue.

After sending to the subscription controller, both also call
`sendToStaleControllers()` to dual-send to any controllers detected as behind
via the canary protocol.

## 2. Controller → Controller: Broadcast

Fire-and-forget. Applied entries are broadcast to all peers via the raft mesh.
If a peer is disconnected, it misses the broadcast. Anti-entropy and
peer-connect snapshots handle recovery.

```mermaid
sequenceDiagram
    participant A as Controller A
    participant B as Controller B
    participant C as Controller C

    A->>B: GossipDelta (broadcast)
    A->>C: GossipDelta (broadcast)
    B->>B: applyDelta
    C->>C: applyDelta
```

## 3. Controller → Controller: Peer Connect Snapshot

When a peer joins or reconnects, the existing controller sends a full snapshot
of all entries (including tombstones). This bootstraps the new peer immediately
rather than waiting for anti-entropy.

```mermaid
sequenceDiagram
    participant Existing as Existing Controller
    participant New as New/Reconnected Controller

    New-->>Existing: (peer connects)
    Existing->>New: GossipDelta (full snapshot)
    New->>New: applyDelta (for each entry)
```

## 4. Controller → Controller: Anti-Entropy

Every 30 seconds, each controller sends a digest to a rotating peer. The peer
responds with entries the sender is missing or has stale.

```mermaid
sequenceDiagram
    participant A as Controller A
    participant B as Controller B

    loop every 30s, rotating peers
        A->>B: GossipDigest (key + version pairs)
        B->>B: compare with local entries
        B->>A: GossipDigestResponse (entries A needs)
        A->>A: applyDelta (for each entry)
    end
```

The digest only contains entries currently in the store. Reaped tombstones
disappear from the digest.

## 5. Router Reconnect: Digest Exchange

When a router connects, the controller sends a digest of entries owned by that
router. The router compares with its current links and responds.

```mermaid
sequenceDiagram
    participant Ctrl as Controller
    participant R as Router

    R-->>Ctrl: (connects, hello includes epoch)
    Ctrl->>Ctrl: HandleRouterEpoch (delete old-epoch entries if epoch changed)
    Ctrl->>Ctrl: ConnectRouter (mark connected, reconcile gossip links)
    Ctrl->>R: GossipDigest (entries owned by this router)

    R->>R: observeVersion (advance clock past digest versions)
    R->>R: build localKeys from current dialed links
    R->>R: tombstone digest entries not in localKeys
    R->>R: include local entries the controller is missing

    R->>Ctrl: GossipDigestResponse (live entries + tombstones)
    Ctrl->>Ctrl: ApplyAndBroadcast (apply, broadcast to peers)
```

The `observeVersion` call ensures tombstones have higher versions than old
entries, even after a restart resets the Lamport clock.

## 6. Canary and Epoch Distribution

The canary carries both the sequence number (for lag detection) and the epoch
(for restart detection). It flows through the canary gossip store, which has no
tombstones and no anti-entropy.

```mermaid
sequenceDiagram
    participant R as Router
    participant Sub as Sub Controller
    participant Peer as Peer Controller

    loop every 5s
        R->>Sub: Canary (seq + epoch)
        Sub->>Sub: canary gossip store: set CanaryValue{Seq, Epoch}
        Sub->>Peer: broadcast canary entry
        Peer->>Peer: canary listener: detect epoch change
    end

    Peer->>Peer: epoch changed? → DeleteByOwnerBefore(routerId, newEpoch)
```

## 7. Stale Controller Detection and Dual-Send

The router's gossip refresher runs every 15 seconds, comparing canary sequences
across controllers. It also detects subscription controller changes.

```mermaid
sequenceDiagram
    participant R as Router
    participant Sub as Sub Controller
    participant Stale as Stale Controller

    R->>R: canary check: stale detected (≥6 ticks behind)

    R->>Stale: GossipDigestRequest
    Stale->>R: GossipDigest (entries owned by router)
    R->>Stale: GossipDigestResponse (corrections)

    Note over R,Stale: subsequent deltas dual-sent until recovery

    R->>Sub: GossipDelta
    R->>Stale: GossipDelta
```

If the subscription controller changes (leader election, reconnect), the
refresher detects it and sends a `GossipDigestRequest` to the new primary.

## 8. Router Restart: Epoch Change

The epoch mechanism cleanly handles router restarts without relying on
tombstones for old entries.

```mermaid
sequenceDiagram
    participant R as Router
    participant C1 as Controller 1
    participant C2 as Controller 2

    Note over R: restart, new epoch generated

    R-->>C1: hello (epoch in headers)
    C1->>C1: HandleRouterEpoch: new epoch > stored epoch
    C1->>C1: DeleteByOwnerBefore(routerId, newEpoch)
    Note over C1: old-epoch link entries deleted

    R->>C1: GossipDelta (new links with new epoch)
    C1->>C1: applyAndBroadcast (entries accepted normally)
    C1->>C2: broadcast new entries

    R->>C1: Canary (seq=1, epoch=new)
    C1->>C2: broadcast canary
    C2->>C2: canary listener: epoch changed
    C2->>C2: DeleteByOwnerBefore(routerId, newEpoch)
    Note over C2: old-epoch entries cleaned up
```

New-epoch entries that arrive at C2 before the canary are safe. The deletion
predicate (`entry.Epoch < newEpoch`) skips entries from the current epoch.

## 9. Tombstone Lifecycle

Within an epoch, link faults are handled by tombstones.

```mermaid
sequenceDiagram
    participant R as Router
    participant C1 as Controller 1
    participant C2 as Controller 2

    Note over R: link fails
    R->>C1: GossipDelta (tombstone with epoch)
    C1->>C1: replace live entry, notify listener (link removed)
    C1->>C2: broadcast tombstone
    C2->>C2: replace live entry, notify listener

    Note over C1,C2: after tombstone TTL (5 min)
    C1->>C1: reapTombstones (entry removed entirely)
    C2->>C2: reapTombstones
```

Tombstone resurrection (where a reaped tombstone allows a stale entry to return
via anti-entropy) is mitigated by the epoch mechanism. Cross-epoch stale
entries are cleaned up by the epoch change handler rather than relying on
tombstones.
