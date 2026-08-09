# Spill-to-Disk Event Streams

## The problem

Several places in ziti hold a stream of events in memory and hope the consumer
keeps up. The router buffers metrics messages for the controllers. The controller
fans events out to WebSocket listeners. In both cases memory is the only backing,
so the limits have to be conservative, and when a consumer falls behind or
disconnects the choice is to grow unbounded or to drop.

Dropping is not always acceptable. Router metrics carry usage data, bucketed as
"circuit x used n bytes in interval y", which feeds billing and cannot be
reconstructed once lost. A meter or histogram losing a few points is fine; a
usage bucket is not.

What we want is a buffer that can spill to disk: keep a bounded amount in memory,
overflow to disk so the effective limit can be much higher, and let a consumer
replay from where it left off after a restart or a reconnect. We still need a
ceiling, but it can be a generous one, and when we do hit it the loss should be
observable rather than silent.

## Isn't this Kafka?

It rhymes with Kafka, and that is the thing to be careful about, because Kafka is
a lot of machinery. But almost all of Kafka's complexity is *distribution*:
partitions, replication, the in-sync-replica protocol, consumer groups,
rebalancing, a coordinator. We have none of that. One process, local disk,
in-process producers and consumers.

Strip distribution and what remains is a segmented write-ahead log: append records
with a length prefix and a checksum, assign each a monotonic offset, roll to a new
file at some size, and reclaim old files once they are no longer needed. That is a
solved, small problem. etcd, BoltDB, SQLite, and badger all have one inside them.

So the conceptual core is genuinely small. The catch is that the easy-sounding
append loop is not where the risk lives.

## Where the real risk lives

Crash consistency, not throughput.

- **Torn tail records.** A crash mid-write leaves a partial record at the end of
  the last segment. On restart we have to scan, detect it (a per-record checksum),
  and truncate to the last good record. Done wrong, we either lose good data or
  replay garbage.
- **fsync policy.** Per-append fsync is correct but slow; batched fsync is fast
  but can lose the un-synced tail on a crash. That is a real choice, and for the
  metrics case it collides with the billing requirement: we cannot tell the
  producer "it is safe to forget this" until it is actually durable.
- **Recovery.** Persisting the committed offset and mapping an offset back to a
  segment and file position across restarts.

The conclusion is to not hand-roll the storage layer. Build on a proven small
segmented-WAL primitive (in Go, `tidwall/wal` is almost exactly the primitive:
write, read-by-index, truncate front and back, segmented) and write only the
policy layer on top. The hard, get-it-wrong part is the storage; the easy part is
the policy, and the policy is the part worth owning.

## The model

One spool is one durable, ordered, bounded stream, with one producer and one
consumer. Many streams means many spools.

That single-consumer constraint is deliberate. It keeps retention tied to a single
commit point, and it is what keeps this from drifting back toward Kafka. The
use cases we care about are all single-consumer once you look closely (see below),
so we lose nothing by enforcing it, and we keep the design honest.

The producer appends records and gets back an offset. The consumer reads forward,
processes, and commits its progress. Retention reclaims records the consumer has
committed, optionally keeping a replay window past the commit point so a consumer
that reconnects can resume from an earlier offset. A hard byte cap bounds the
whole thing, with an explicit policy for what happens when it is reached.

## Core interface

```go
// Package spool provides a durable, ordered, bounded event stream that spills to
// disk, so a buffer can outlive a long consumer outage or a process restart.
package spool

// Spool is a single durable stream: one producer appends, one consumer reads and
// commits. Storage is a segmented log on disk; reclamation frees whole segments.
type Spool struct{ /* ... */ }

// Offset is a record's monotonically increasing position. Stable across restarts,
// never reused.
type Offset uint64

// Record is one entry read from a spool.
type Record struct {
	Offset Offset
	Data   []byte
}

// Open opens or creates the spool rooted at dir, recovering existing records and
// the committed offset from a previous run.
func Open(dir string, opts Options) (*Spool, error)

func (s *Spool) Close() error

type Options struct {
	// MaxBytes bounds total on-disk size; 0 means unbounded.
	MaxBytes int64

	// Overflow decides what happens when an append would exceed MaxBytes.
	Overflow OverflowPolicy

	// RetainAfterCommit keeps this many bytes of already-committed records for
	// replay, so a consumer can resume from an earlier offset after a reconnect.
	// 0 reclaims on commit (a pure queue).
	RetainAfterCommit int64

	// Sync trades durability against throughput for appends.
	Sync          SyncMode
	FlushInterval time.Duration // flush cadence when Sync is SyncBatched

	// SegmentBytes is the rollover size; reclamation frees whole segments.
	SegmentBytes int64
}

type OverflowPolicy int

const (
	Block      OverflowPolicy = iota // backpressure: Append waits for room
	DropOldest                       // reclaim oldest, even if uncommitted (lossy; counted in Stats)
	Reject                           // fail the append with ErrFull, spool unchanged
)

type SyncMode int

const (
	SyncAlways  SyncMode = iota // fsync before Append returns: no loss on crash, slower
	SyncBatched                 // batch fsyncs: faster, may lose the un-synced tail on crash
)

// Append durably writes one record and returns its assigned offset. It blocks
// only under OverflowPolicy Block, and only until room is reclaimed or ctx ends.
func (s *Spool) Append(ctx context.Context, data []byte) (Offset, error)

// Reader returns the consumer cursor, positioned just after the last committed
// offset (or at the earliest retained record on first use). One active reader.
func (s *Spool) Reader() *Reader

// Next returns the next record, blocking until one is available or ctx ends. It
// advances the read position only; progress is not durable until Commit.
func (r *Reader) Next(ctx context.Context) (Record, error)

// Seek repositions the cursor to off, for resume-from-index. ErrGap if off has
// already been reclaimed.
func (r *Reader) Seek(off Offset) error

// Commit durably marks everything up to and including off as consumed, releasing
// records below off (less RetainAfterCommit) for reclamation.
func (r *Reader) Commit(off Offset) error

type Stats struct {
	Earliest  Offset // oldest retained
	Next      Offset // offset the next Append will receive
	Committed Offset // last durably committed
	SizeBytes int64
	Dropped   uint64 // records reclaimed before commit (DropOldest)
}

func (s *Spool) Stats() Stats

var (
	ErrGap    = errors.New("spool: offset reclaimed")
	ErrFull   = errors.New("spool: at capacity")
	ErrClosed = errors.New("spool: closed")
)
```

## Use cases

### Router to controller metrics (first, and the reason to build it)

This is a durable outbound queue. One producer (the metrics collection loop), one
consumer (the loop that sends to a controller and retries until delivered),
retention until delivered. It is the use case with the hardest requirement, no
silent loss of usage data, so it is the one that proves the durability story.

```go
// collection side: fast, durable, bounded; never waits on a controller
off, err := s.Append(ctx, msg.Marshal())

// drain side: owns the send, the retry, and the controller narrowing
r := s.Reader()
for {
	rec, err := r.Next(ctx)          // next uncommitted record
	if err != nil { return }
	for sendToTargets(rec.Data) != nil {
		select {                     // retry the SAME record until delivered
		case <-ctx.Done(): return
		case <-time.After(backoff):
		}
	}
	_ = r.Commit(rec.Offset)         // retain-until-delivered, now disk-backed
}
```

Today the router holds undelivered metrics in an in-memory slice, which bounds how
long a disconnect it can survive and risks unbounded growth. A spool replaces that
slice with a disk-backed queue: it survives restarts and tolerates a much longer
disconnect, with a bounded, observable failure mode when the cap is finally hit.

It cannot make loss impossible. A hard `MaxBytes` means a long-enough outage forces
a choice. For billing that choice is `Block` (backpressure) or a generous cap plus
alerting on `Stats.Dropped` and on lag (`Next - Committed`); never silent
`DropOldest`. The spool buys a much longer tolerable disconnect, not an infinite
one, and it makes the loss observable when it finally happens.

### WebSocket event feeds (deferred)

Single consumer per feed. A client registers a named endpoint (create-if-not-
exist), streams events, and on a drop reconnects with the same id and its last
index. The server resumes with `Seek`. Set `RetainAfterCommit` to a replay window
so a reconnecting client can catch up; a client that waited too long gets `ErrGap`
and resyncs.

This is the same primitive as the metrics queue plus a replay window, so the core
already covers it. What it adds is a feed registry, a reconnect handshake, and
orphan GC: a named durable feed whose consumer never returns is disk that nobody
is draining, and reaping it is a new lifecycle concern the metrics case does not
have. That layer is deferred until there is a concrete ask.

Whether to build it at all is a genuine product call. Pushing events to a shared
queue over AMQP, which we already support, is simpler for us and powerful. The
embedded feed's value is purely the no-broker story for users who want minimal
infrastructure. Reasonable people can disagree; it is worth doing only if that
story proves worth the lifecycle complexity.

### Cross-controller aggregation (leave to AMQP)

A WS consumer that wants every controller's events, with other controllers
forwarding to the one hosting the feed, is where the embedded approach starts
re-growing the distribution problem we were glad to avoid: who is the aggregator,
what happens on failover, where the durable feed lives, how forwards survive a
partition. That is exactly AMQP's sweet spot. A user who needs everything
aggregated is a user for whom "point a broker at it" is a fine answer. We do not
build this embedded.

## Scope

In scope, now:

- The `spool` core: a single-stream, single-consumer, disk-backed offset log with
  commit-based retention, a replay window, a byte cap, and an overflow policy,
  built on a proven WAL primitive.
- Its first user: the router to controller metrics queue.

Out of scope, deferred or declined:

- Multi-consumer fan-out, a named-feed registry, and orphan GC (deferred with the
  WebSocket feed).
- Any cross-controller forwarding (declined; use AMQP).

Note that the spool is independent of the metrics-over-gossip rollout. Narrowing
the metrics message to a single controller (gossip rollout phase 2) is a small,
separate change to the recipient set. Swapping the in-memory queue for a spool is
a distinct, later enhancement to the reporting loop. They compose, but they are
not the same change and should not ship in the same PR.

