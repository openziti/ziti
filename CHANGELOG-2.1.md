# Release 2.1.0

## What's New

### Gossip-Based Link Propagation

Link state is now propagated between routers and controllers using a gossip protocol
instead of each router reporting links only to its connected controller. This means all
controllers in an HA cluster have a consistent view of the network's link topology,
enabling better routing decisions and faster convergence after failover.

* [Issue #3726](https://github.com/openziti/ziti/issues/3726) - Use gossip to propagate link data between routers and controllers

### Fully Connected Controller Mesh

Controllers now proactively maintain connections to all cluster members using a peer
dialer with exponential backoff. Previously, if a controller lost its connection to a
peer, it relied on the peer to reconnect. Now both sides actively dial, with
deterministic tie-breaking to resolve duplicate connections.

* [Issue #3684](https://github.com/openziti/ziti/issues/3684) - Keep controller mesh fully connected

### Router and Peer Event Pools

Router connection setup and gossip message processing now run in bounded worker pools
instead of inline on the channel receive path. This prevents thundering herd reconnections
and gossip lock contention from starving heartbeat processing, which previously caused
cascading disconnects.

Two pools split work by connection type so router floods can't starve peer processing:

- **Router events pool**: handles ConnectRouter, gossip deltas/digests from routers, and
  canary messages. If the pool is full when a router connects, the connection is rejected.
  For gossip messages, the message is dropped silently (anti-entropy recovers).
- **Peer events pool**: handles gossip deltas/digests/responses from peer controllers.
  Messages are dropped when the pool is full.

New configuration under `network`:

| Key | Default | Description |
|-----|---------|-------------|
| `routerEventsPool.queueSize` | `1` | Work queue size. Kept small so excess work is rejected fast. |
| `routerEventsPool.maxWorkers` | `200` | Max concurrent router event workers. |
| `peerEventsPool.queueSize` | `1` | Work queue size for peer events. |
| `peerEventsPool.maxWorkers` | `10` | Max concurrent peer event workers. |

New metrics (via goroutine pool metrics):

| Metric prefix | Description |
|---------------|-------------|
| `pool.router.events.*` | Router events pool: `current_size`, `busy_workers`, `work_timer`, `queue_size` |
| `pool.peer.events.*` | Peer events pool: same metrics |

### Bug Fixes

* [Issue #3746](https://github.com/openziti/ziti/issues/3746) - Fix connect events handler goroutine leak
