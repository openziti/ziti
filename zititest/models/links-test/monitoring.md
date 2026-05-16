# Links-Test Monitoring Criteria

This document is the post-run analysis playbook for the `links-test` fablab
scenario. It catalogs the metrics emitted by the controllers and routers,
and lists symptoms to scan for and what they probably mean.

Use it two ways:

1. **Live**: tail metrics during a run, watch for the symptoms below.
2. **Post-mortem**: after a failure (or even a clean pass), load this file
   plus the captured metrics into an agent and ask it to scan for matches.

When new failure modes turn up, add entries here. The goal is that each
production incident leaves the playbook a little better than it found it.

## What is captured

### Controller-side, via `metrics.Registry`

**Pool metrics** (one set per pool, prefix shown):

| Pool                       | Prefix                  | What it does                                                          |
|----------------------------|-------------------------|-----------------------------------------------------------------------|
| Router connect             | `pool.router.connect`   | Accepts new router ctrl-channel binds                                 |
| Router events              | `pool.router.events`    | Processes per-router messages (metrics, link reports, etc.)           |
| Peer events                | `pool.peer.events`      | Processes incoming gossip from peer controllers                       |
| Router messaging           | `pool.router.messaging` | Outbound messaging to routers                                         |

Each pool exposes:

- `<prefix>.queue_size` (gauge) - work waiting for a worker
- `<prefix>.worker_count` (gauge) - workers currently alive (including idle)
- `<prefix>.busy_workers` (gauge) - workers currently executing
- `<prefix>.work_timer` (timer) - per-task execution time histogram and rates

**Gossip store metrics**:

- `gossip.pending_acks` (gauge) - in-flight `SetConfirmed` waits
- `gossip.<type>.owners` (gauge) - distinct owners with state
- `gossip.<type>.entries.live` (gauge) - non-tombstone entries
- `gossip.<type>.entries.tombstones` (gauge) - tombstone entries waiting to age out
- `gossip.<type>.owners.drained` (gauge) - dropped owners awaiting compaction
- `gossip.<type>.delta.received` (meter) - entries arriving from peers or routers
- `gossip.<type>.delta.applied` (meter) - entries that passed the version check
- `gossip.<type>.delta.rejected_stale` (meter) - entries rejected (older version or drained owner)
- `gossip.<type>.broadcast.sent` (meter) - broadcasts initiated locally
- `gossip.<type>.anti_entropy.owners_matched` (meter) - owners short-circuited via owner-hash match on incoming digest
- `gossip.<type>.anti_entropy.owners_diffed` (meter) - owners requiring per-entry comparison (hash mismatch)

Today `<type>` is `links` and `canary`.

### Router-side

Routers stream their own metrics into the controller they're connected to,
so the controller-side metrics include the router pool metrics under the
router's source id. Use the per-router breakdown to spot one bad router
vs. a system-wide issue.

### Host- and process-level (in-process)

Enabled per-process via the `hostMetrics: { enabled: true }` config stanza
(controller: `network.hostMetrics`, router: `metrics.hostMetrics`). Off by
default. **Linux only** - on other operating systems the controller and
router log a one-time warning and register nothing, since the data is
populated from `/proc`. Once enabled, the controller and each router emit:

- `process.goroutines` (gauge)
- `host.cpu.percent` (gauge, float)
- `host.load.1m` / `5m` / `15m` (gauges, float; Linux only)
- `host.mem.total` / `used` / `available` (gauges, bytes)
- `host.mem.used_percent` (gauge, float)
- `host.disk.read_bytes` / `write_bytes` (gauges, cumulative)
- `host.disk.available_bytes` (gauge, free bytes on `/`)
- `host.disk.used_percent` (gauge, float)
- `host.net.rx_bytes` / `tx_bytes` (gauges, cumulative)
- `host.net.rx_drops` / `tx_drops` (gauges, cumulative)

Router host stats stream up through the existing controller-bound metrics
channel, so all of them land in the controller-side dump under the source
router's id.

### Host-level (external fallback)

`sar -A 5 -o /var/log/ziti/sar.bin` running as a separate process on each
host. Use this when the in-process metrics stop (controller crash, OOM
kill) and you need to know what the kernel was seeing in the last moments.

## How to read it

The signals below are organized as **symptom -> signal -> probable cause**.
Most matter only when sustained: a single one-second spike during reconciliation
is normal, a minute of the same condition is not.

### Pool saturation

| Signal                                                 | Threshold                       | Probable cause                                                                                             |
|--------------------------------------------------------|---------------------------------|------------------------------------------------------------------------------------------------------------|
| `pool.router.connect.queue_size`                       | > 80% of QueueSize for > 30s    | Router reconnect storm exceeding `MaxWorkers`. Symptom that bit us pre-split.                              |
| `pool.router.events.queue_size`                        | > 80% of QueueSize for > 30s    | Slow event handler, or a router sending too much. Look at per-router work_timer.                           |
| `pool.peer.events.queue_size`                          | sustained > 0                   | Peer controller is sending faster than we can apply. May indicate a gossip storm (see broadcast section).  |
| `pool.*.work_timer` p99                                | climbing over a run             | Handler slowdown - DB contention, lock starvation, GC pause. Cross-check `host.cpu.percent`.               |
| `pool.*.busy_workers`                                  | at MaxWorkers sustained         | Either real load or a single stuck task pinning a worker. Inspect for goroutine leak via `process.goroutines`. |

### Gossip convergence

| Signal                                                                | Threshold                       | Probable cause                                                                                                 |
|-----------------------------------------------------------------------|---------------------------------|----------------------------------------------------------------------------------------------------------------|
| `gossip.<type>.delta.rejected_stale` rate                             | > `delta.applied` rate          | Peers fighting over versions. Suspect clock anomaly, replay, or owner drain race.                              |
| `gossip.<type>.broadcast.sent` rate                                   | >> known write rate             | Rebroadcast storm. Look for an entry being re-set repeatedly, or anti-entropy loop converging slowly.          |
| `gossip.<type>.entries.live` across controllers                       | differs > 1% during quiet       | Convergence broken. One controller is missing entries another has.                                             |
| `gossip.<type>.owners` across controllers                             | differs                         | One controller never learned about an owner, or applied `DropOwner` and the others didn't.                     |
| `gossip.pending_acks`                                                 | > 0 for more than a few seconds | `SetConfirmed` waiting on a peer that isn't acking. Peer slow, queue full, or disconnect-cleanup not firing.   |
| `gossip.<type>.delta.received` rate                                   | spikes at `AntiEntropyInterval` | Peers are perpetually catching up via anti-entropy. The direct-broadcast path is dropping or lagging.          |

### Memory and lifecycle

| Signal                                              | Threshold                                  | Probable cause                                                                                                                 |
|-----------------------------------------------------|--------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------|
| `gossip.<type>.owners`                              | strictly increasing over a long run        | `DropOwner` not firing, or wrong owner identifier. Check `HandleRouterDelete` is reaching us.                                  |
| `gossip.<type>.entries.tombstones` / `entries.live` | > 10x sustained                            | Reaper falling behind, or `TombstoneTTL` too long. Confirm reaper goroutine is alive.                                          |
| `gossip.<type>.owners.drained`                      | > 0 sustained                              | Compaction lag - drained owners exist but `RemoveCb` predicate is not firing. May indicate tombstones still being repopulated. |
| SAR memory used (RSS)                               | climbing while `entries.live` stays flat   | Leak outside the gossip store. Could be goroutine leak, channel buffers, event subscribers.                                    |
| SAR memory used                                     | climbing with `entries.live`               | Expected if the test is adding state; suspicious if entries should be steady-state.                                            |

### Convergence latency (qualitative)

We don't have a direct convergence-lag histogram yet. Approximations:

- After a quiet period (no link churn), `entries.live` should be equal across
  all controllers. If it isn't, propagation is lossy.
- After a burst, the slowest controller's `entries.live` should converge with
  the leader's within ~`AntiEntropyInterval`. If it takes much longer, the
  direct broadcast path is failing for that controller.

### Host-level

| Signal                                  | Threshold                  | Probable cause                                                                |
|-----------------------------------------|----------------------------|-------------------------------------------------------------------------------|
| `host.cpu.percent`                      | > 80% sustained            | CPU pressure; cross-check `pool.*.work_timer` for which path is hot.          |
| `host.load.1m`                          | > num_cpus sustained       | Run queue building up - process scheduling delays.                            |
| `host.mem.used_percent`                 | > 80%                      | About to OOM. Cross-reference `gossip.*.owners` trend.                        |
| `host.disk.used_percent`                | > 85%                      | About to fill disk - controller will crash on next bolt write. See "log-volume runaway" pattern. |
| `host.disk.available_bytes`             | falling fast (GB/min)      | Log or DB growth blew up; rate is what catches it before the absolute threshold does. |
| `host.net.rx_drops` or `tx_drops`       | rising fast                | Kernel-level packet drops (NIC ring / socket buffer). TCP retransmits cover correctness, so this is a latency/load signal, not a correctness one. Worry about it if rates climb sharply or correlate with elevated `pool.*.work_timer` p99. |
| `process.goroutines`                    | climbing over a run        | Goroutine leak somewhere - likely a channel or context not being closed.      |
| SAR iowait (external)                   | > 20% sustained            | Disk contention; in-process metrics can't see this directly.                  |

## Cross-cutting heuristics

A few patterns to look for that combine signals:

- **The "everything is fine but the test failed" pattern**: pools are quiet,
  gossip looks healthy, but the test reports unconverged links. Check
  per-controller `gossip.links.entries.live` and `owners` - the failure may
  be a single-controller divergence that the aggregate metrics hide.

- **The "pool A is saturated because pool B is slow" pattern**: two pools
  saturate together. Look at `work_duration` - the upstream pool is usually
  the one whose tasks are blocking on the downstream pool.

- **The "tombstone storm" pattern**: `entries.tombstones` spikes, then
  `broadcast.sent` and `delta.received` spike in lockstep across the mesh.
  Caused by a mass `DropOwner` (router scaledown) propagating. Expected;
  only investigate if it doesn't subside within `TombstoneTTL`.

- **The "ghost owner" pattern**: `gossip.<type>.owners` is higher than the
  count of live routers. Probably a router that disconnected without
  triggering `HandleRouterDelete`. Will compact eventually if `DropOwner`
  fires; otherwise grows unboundedly.

- **The "log-volume runaway" pattern**: `host.disk.available_bytes` drops by
  hundreds of MB per minute while `gossip.*` and `pool.*` look fine.
  Symptom is rapid log rotation (many `*-<timestamp>.log` files under
  `~/logs/`). Burned us on the first instrumented run: a wide-open
  `metricFilter: .*` in the events config emitted per-link histograms for
  every router every poll, filling the 6.8GB root disk in minutes. The
  controller then could not write its bolt DB and crashed; restart left it
  returning HTTP 500 on `/authenticate`. Fix: explicit metric allowlist in
  the events config (see `configs/ctrl.yml.tmpl`), and never let the metric
  stream include `link.*` / `router.*` / `service.*` series at full
  cardinality.

## Adding new criteria

When a run surfaces a failure mode not covered above, add an entry. Keep the
format symptom -> signal -> probable cause, and prefer specific thresholds
to vague ones - the next reader (human or agent) needs something to match
against.
