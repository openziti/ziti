/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

// AsyncHandler is a bounded, async slog.Handler that hands records to a single
// drain goroutine and onto a downstream handler. Records at or above the
// configured BlockThreshold block when the queue is full; records below it
// drop and are counted toward a periodic summary line.
//
// AsyncHandler is the root of the slog handler chain: WithAttrs returns a
// boundHandler that prepends bound attrs to records, and WithGroup returns a
// groupedHandler that wraps record attrs in slog.Group(name, ...). Both
// wrappers delegate back to AsyncHandler.Handle, which is where the queueing
// and drain dispatch live.
type AsyncHandler struct {
	opts         AsyncOptions
	downstream   slog.Handler
	queue        chan queuedRecord
	closeNotify  chan struct{}
	drainDone    chan struct{}
	closed       atomic.Bool
	downstreamMu sync.Mutex
	dropCounts   [7]atomic.Int64
	drainErrors  atomic.Int64
	// windowStart is the start of the current summary window. It is only
	// accessed by the drain goroutine after the handler is constructed.
	windowStart time.Time
}

// queuedRecord carries the originating call's context alongside the record so
// downstream handlers that consult ctx values (tracing, OTel) see them across
// the async hop.
type queuedRecord struct {
	ctx    context.Context
	record slog.Record
}

// NewAsyncHandler returns an AsyncHandler that drains records onto downstream.
// It validates opts and starts the drain goroutine; on validation failure it
// returns an error and no resources are leaked.
func NewAsyncHandler(downstream slog.Handler, opts AsyncOptions) (*AsyncHandler, error) {
	if downstream == nil {
		return nil, errors.New("downstream handler must not be nil")
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	h := &AsyncHandler{
		opts:        opts,
		downstream:  downstream,
		queue:       make(chan queuedRecord, opts.QueueSize),
		closeNotify: make(chan struct{}),
		drainDone:   make(chan struct{}),
		windowStart: time.Now(),
	}
	go h.drain()
	return h, nil
}

// Enabled returns true. AsyncHandler does no level gating itself; records are
// gated before they reach it. Direct slog callers go through the named-logger
// registry's handler, whose Enabled checks the per-name override or live
// global level, and bridged logrus records are pre-filtered by logrus's own
// level check (see doc/design/logging-refactor.md). A second check here would
// be a redundant atomic load on every call.
func (h *AsyncHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

// Handle enqueues r for the drain goroutine. Records at or above
// BlockThreshold block when the queue is full (with a closeNotify escape so
// shutdown can't deadlock); records below BlockThreshold drop and increment
// the per-level drop counter.
func (h *AsyncHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.closed.Load() {
		return nil
	}
	qr := queuedRecord{ctx: ctx, record: r}
	if r.Level >= h.opts.BlockThreshold {
		select {
		case h.queue <- qr:
		case <-h.closeNotify:
			// shutdown; best-effort drop
		}
	} else {
		select {
		case h.queue <- qr:
		default:
			h.dropCounts[dropIdx(r.Level)].Add(1)
		}
	}
	return nil
}

// WithAttrs returns a child handler that prepends the given attrs to every
// record it sees before forwarding to AsyncHandler. An empty attrs slice
// returns the receiver unchanged so slog.Logger.With() with no args is free.
func (h *AsyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return &boundHandler{parent: h, attrs: slices.Clone(attrs)}
}

// WithGroup returns a child handler that wraps every record's attrs in
// slog.Group(name, ...) before forwarding. An empty group name returns the
// receiver unchanged, matching slog's convention.
func (h *AsyncHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &groupedHandler{parent: h, name: name}
}

// Close initiates shutdown and returns immediately. The drain goroutine
// finishes processing whatever is already enqueued, emits a final drop
// summary if any drops occurred, then exits and closes the drainDone channel.
// Calling Close more than once is a no-op.
//
// A producer that races with Close may still successfully enqueue a record
// that the drain never sees; per the design, logging during shutdown is
// best-effort. Records routed through SyncEmit are durable because they
// bypass the queue and write synchronously.
func (h *AsyncHandler) Close() error {
	if !h.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(h.closeNotify)
	return nil
}

// SyncEmit flushes the records currently queued, then writes r through the
// downstream handler synchronously on the caller's goroutine. It exists so
// that fatal/panic records reach the downstream before the process exits, and
// flushing first means the buffered context leading up to the fatal/panic
// (the very records an operator wants in a post-mortem) lands ahead of it
// rather than being lost when the process exits out from under the drain.
//
// The flush is bounded and non-blocking, so a producer still enqueuing cannot
// make it spin and the drain goroutine cannot deadlock it. A single record the
// drain has already pulled but not yet written may still land after r; ordering
// on the exit path is best-effort, not exact.
func (h *AsyncHandler) SyncEmit(ctx context.Context, r slog.Record) error {
	h.downstreamMu.Lock()
	defer h.downstreamMu.Unlock()
	h.flushQueuedLocked()
	return h.downstream.Handle(ctx, r)
}

// flushQueuedLocked drains the records sitting in the queue through the
// downstream handler. The caller must hold downstreamMu. It is bounded by the
// queue capacity and stops at the first empty receive, so a producer still
// enqueuing cannot make it spin, and the drain goroutine (which may be parked
// on downstreamMu) cannot deadlock it. Records the concurrent drain pulls
// first are written by the drain; the rest are written here, all under the
// same mutex so no downstream.Handle calls interleave.
func (h *AsyncHandler) flushQueuedLocked() {
	for i := cap(h.queue); i > 0; i-- {
		select {
		case qr := <-h.queue:
			h.handleLocked(qr.ctx, qr.record)
		default:
			return
		}
	}
}

// drain is the single goroutine that pulls records off the queue and calls
// the downstream handler. It also emits the periodic drop-summary record on
// the summary ticker, and on shutdown does a final-flush pass through any
// remaining queued records plus a final summary.
func (h *AsyncHandler) drain() {
	defer close(h.drainDone)
	ticker := time.NewTicker(h.opts.SummaryInterval)
	defer ticker.Stop()

	for {
		select {
		case qr := <-h.queue:
			h.dispatch(qr.ctx, qr.record)
		case <-ticker.C:
			h.emitSummaryIfAny()
		case <-h.closeNotify:
			// final flush: drain any records that beat the close-notify
			// reception, then emit a final summary if anything was dropped.
			for {
				select {
				case qr := <-h.queue:
					h.dispatch(qr.ctx, qr.record)
				default:
					h.emitSummaryIfAny()
					return
				}
			}
		}
	}
}

// dispatch hands one record to the downstream handler under downstreamMu, the
// same mutex SyncEmit takes, so the downstream handler does not need to be
// concurrency-safe on its own.
func (h *AsyncHandler) dispatch(ctx context.Context, r slog.Record) {
	h.downstreamMu.Lock()
	defer h.downstreamMu.Unlock()
	h.handleLocked(ctx, r)
}

// handleLocked hands one record to the downstream handler, counting and
// reporting a downstream error. The caller must hold downstreamMu. On error it
// bumps drainErrors and writes once to os.Stderr, bypassing slog to avoid
// recursion if the downstream handler is the thing failing.
func (h *AsyncHandler) handleLocked(ctx context.Context, r slog.Record) {
	if err := h.downstream.Handle(ctx, r); err != nil {
		h.drainErrors.Add(1)
		fmt.Fprintf(os.Stderr, "logging: downstream handler error: %v\n", err)
	}
}

// emitSummaryIfAny snapshots and zeroes the drop and drain-error counters,
// and if anything was non-zero emits a single summary record. Runs only from
// the drain goroutine.
func (h *AsyncHandler) emitSummaryIfAny() {
	var counts [7]int64
	anything := false
	for i := range h.dropCounts {
		c := h.dropCounts[i].Swap(0)
		counts[i] = c
		if c > 0 {
			anything = true
		}
	}
	errs := h.drainErrors.Swap(0)
	if errs > 0 {
		anything = true
	}
	if !anything {
		return
	}

	now := time.Now()
	since := h.windowStart
	h.windowStart = now

	r := slog.NewRecord(now, slog.LevelWarn, "logging queue full, message drop summary", 0)
	for i, c := range counts {
		if c > 0 {
			r.AddAttrs(slog.Int64(canonicalLevelNames[i], c))
		}
	}
	if errs > 0 {
		r.AddAttrs(slog.Int64("drain_errors", errs))
	}
	r.AddAttrs(slog.Time("since", since))

	h.downstreamMu.Lock()
	_ = h.downstream.Handle(context.Background(), r)
	h.downstreamMu.Unlock()
}

// canonicalLevelNames maps the drop-counter index to its canonical lowercase
// level name, used as the attr key in the summary record.
var canonicalLevelNames = [7]string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}

// dropIdx maps a slog.Level to its drop-counter bucket. Non-canonical levels
// (e.g. slog.LevelDebug+1) bucket into the canonical level whose value they
// most recently exceeded, so every record contributes to exactly one bucket.
func dropIdx(l slog.Level) int {
	switch {
	case l < slog.LevelDebug:
		return 0 // trace
	case l < slog.LevelInfo:
		return 1 // debug
	case l < slog.LevelWarn:
		return 2 // info
	case l < slog.LevelError:
		return 3 // warn
	case l < LevelFatal:
		return 4 // error
	case l < LevelPanic:
		return 5 // fatal
	default:
		return 6 // panic
	}
}
