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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// recordingHandler captures every record passed to Handle so tests can
// inspect what made it through the async sink.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

func (h *recordingHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.records...)
}

// blockingHandler holds Handle until release is closed, signaling the first
// entry into Handle via entered. It's used to deterministically park the
// drain inside a downstream call so the queue fills up.
type blockingHandler struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	inner   *recordingHandler
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		inner:   &recordingHandler{},
	}
}

func (h *blockingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *blockingHandler) Handle(ctx context.Context, r slog.Record) error {
	h.once.Do(func() { close(h.entered) })
	<-h.release
	return h.inner.Handle(ctx, r)
}
func (h *blockingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *blockingHandler) WithGroup(string) slog.Handler      { return h }

// erroringHandler returns a fixed error from every Handle call. Used to
// exercise the drain's error-counting path.
type erroringHandler struct {
	err   error
	count atomic.Int64
}

func (h *erroringHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *erroringHandler) Handle(context.Context, slog.Record) error {
	h.count.Add(1)
	return h.err
}
func (h *erroringHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *erroringHandler) WithGroup(string) slog.Handler      { return h }

func makeRecord(level slog.Level, msg string) slog.Record {
	return slog.NewRecord(time.Now(), level, msg, 0)
}

func TestNewAsyncHandlerValidatesOptions(t *testing.T) {
	rec := &recordingHandler{}
	_, err := NewAsyncHandler(rec, AsyncOptions{}) // zero values invalid
	require.Error(t, err)
}

func TestNewAsyncHandlerRejectsNilDownstream(t *testing.T) {
	_, err := NewAsyncHandler(nil, DefaultOptions())
	require.Error(t, err)
}

func TestAsyncHandlerNormalFlow(t *testing.T) {
	rec := &recordingHandler{}
	h, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelInfo, fmt.Sprintf("msg %d", i))))
	}

	require.NoError(t, h.Close())
	<-h.drainDone

	require.Equal(t, 5, rec.count())
}

// TestAsyncHandlerDropsSubThresholdWhenFull parks the drain inside the
// downstream so the queue fills, then verifies that sub-threshold records
// drop and bump the per-level counter without blocking the caller.
func TestAsyncHandlerDropsSubThresholdWhenFull(t *testing.T) {
	block := newBlockingHandler()
	opts := DefaultOptions()
	opts.QueueSize = 2
	opts.SummaryInterval = time.Hour
	h, err := NewAsyncHandler(block, opts)
	require.NoError(t, err)

	// Send one record; wait for the drain to enter the downstream.
	require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelDebug, "m1")))
	<-block.entered

	// Fill the queue (capacity 2).
	require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelDebug, "m2")))
	require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelDebug, "m3")))

	// Sub-threshold records now drop without blocking.
	for i := 0; i < 5; i++ {
		require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelDebug, "dropped")))
	}
	require.Equal(t, int64(5), h.dropCounts[dropIdx(slog.LevelDebug)].Load())

	// Release the downstream, close, and verify only the original three got
	// through. Filter the close-flush drop-summary out of the count.
	close(block.release)
	require.NoError(t, h.Close())
	<-h.drainDone

	nonSummary := 0
	for _, r := range block.inner.snapshot() {
		if r.Message != "logging queue full, message drop summary" {
			nonSummary++
		}
	}
	require.Equal(t, 3, nonSummary)
}

// TestAsyncHandlerBlocksAtThreshold proves a record at the block threshold
// parks Handle until the drain frees a slot, instead of dropping.
func TestAsyncHandlerBlocksAtThreshold(t *testing.T) {
	block := newBlockingHandler()
	opts := DefaultOptions()
	opts.QueueSize = 2
	opts.SummaryInterval = time.Hour
	h, err := NewAsyncHandler(block, opts)
	require.NoError(t, err)

	require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelInfo, "m1")))
	<-block.entered
	require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelInfo, "m2")))
	require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelInfo, "m3")))

	done := make(chan struct{})
	go func() {
		defer close(done)
		require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelWarn, "warn1")))
	}()

	select {
	case <-done:
		t.Fatal("Handle on Warn returned but should have blocked")
	case <-time.After(50 * time.Millisecond):
	}

	close(block.release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Handle on Warn never returned after the downstream was released")
	}

	require.NoError(t, h.Close())
	<-h.drainDone
	require.Equal(t, 4, block.inner.count())
}

// TestAsyncHandlerCloseUnblocksHandle proves a Handle call parked on the
// blocking arm of select returns when Close fires, instead of deadlocking.
func TestAsyncHandlerCloseUnblocksHandle(t *testing.T) {
	block := newBlockingHandler()
	defer close(block.release)
	opts := DefaultOptions()
	opts.QueueSize = 1
	opts.SummaryInterval = time.Hour
	h, err := NewAsyncHandler(block, opts)
	require.NoError(t, err)

	require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelInfo, "m1")))
	<-block.entered
	require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelInfo, "m2")))

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = h.Handle(context.Background(), makeRecord(slog.LevelWarn, "warn"))
	}()

	select {
	case <-done:
		t.Fatal("Handle should be blocked at this point")
	case <-time.After(50 * time.Millisecond):
	}

	require.NoError(t, h.Close())

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock Handle")
	}
}

func TestAsyncHandlerCloseIdempotent(t *testing.T) {
	rec := &recordingHandler{}
	h, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, h.Close())
	require.NoError(t, h.Close())
}

func TestAsyncHandlerHandleAfterCloseReturnsNil(t *testing.T) {
	rec := &recordingHandler{}
	h, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, h.Close())
	<-h.drainDone

	require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelInfo, "after-close")))
}

// TestAsyncHandlerHandleRacingCloseNoPanic spawns many producers and a
// concurrent Close. The handler must never panic and must drain cleanly.
func TestAsyncHandlerHandleRacingCloseNoPanic(t *testing.T) {
	rec := &recordingHandler{}
	h, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = h.Handle(context.Background(), makeRecord(slog.LevelInfo, "x"))
			}
		}()
	}

	time.Sleep(time.Millisecond) // let producers ramp
	require.NoError(t, h.Close())
	wg.Wait()
	<-h.drainDone
}

// TestAsyncHandlerSummaryEmittedOnTick seeds the drop counters directly and
// waits for the next ticker emission. Using direct counter seeding avoids
// timing flakes around when the queue actually fills.
func TestAsyncHandlerSummaryEmittedOnTick(t *testing.T) {
	rec := &recordingHandler{}
	opts := DefaultOptions()
	opts.SummaryInterval = 20 * time.Millisecond
	h, err := NewAsyncHandler(rec, opts)
	require.NoError(t, err)

	h.dropCounts[dropIdx(slog.LevelDebug)].Add(7)
	h.dropCounts[dropIdx(slog.LevelInfo)].Add(3)

	require.Eventually(t, func() bool { return rec.count() >= 1 }, time.Second, 5*time.Millisecond)

	var summary *slog.Record
	for _, r := range rec.snapshot() {
		if r.Message == "logging queue full, message drop summary" {
			r := r
			summary = &r
			break
		}
	}
	require.NotNil(t, summary, "expected drop-summary record")

	attrs := map[string]any{}
	summary.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	require.Equal(t, int64(7), attrs["debug"])
	require.Equal(t, int64(3), attrs["info"])
	require.NotContains(t, attrs, "trace", "zero-count levels must not appear")
	require.Contains(t, attrs, "since")

	require.NoError(t, h.Close())
	<-h.drainDone
}

// TestSyncEmitDeliversSynchronously proves SyncEmit calls downstream.Handle
// on the caller's goroutine, bypassing the queue.
func TestSyncEmitDeliversSynchronously(t *testing.T) {
	rec := &recordingHandler{}
	h, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)
	defer func() {
		_ = h.Close()
		<-h.drainDone
	}()

	require.NoError(t, h.SyncEmit(context.Background(), makeRecord(LevelFatal, "fatal!")))
	require.Equal(t, 1, rec.count())
	require.Equal(t, "fatal!", rec.snapshot()[0].Message)
}

// TestSyncEmitSerializesWithDrain holds the drain inside the downstream, then
// fires a SyncEmit. SyncEmit must wait for the in-flight drain dispatch to
// finish (they share downstreamMu), so the queued record lands before the
// SyncEmit record.
func TestSyncEmitSerializesWithDrain(t *testing.T) {
	block := newBlockingHandler()
	h, err := NewAsyncHandler(block, DefaultOptions())
	require.NoError(t, err)

	require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelInfo, "queued")))
	<-block.entered

	syncDone := make(chan struct{})
	go func() {
		defer close(syncDone)
		require.NoError(t, h.SyncEmit(context.Background(), makeRecord(LevelFatal, "sync")))
	}()

	// SyncEmit must be blocked waiting for downstreamMu held by the drain.
	select {
	case <-syncDone:
		t.Fatal("SyncEmit returned while the drain was still inside Handle")
	case <-time.After(50 * time.Millisecond):
	}

	close(block.release)
	<-syncDone

	require.NoError(t, h.Close())
	<-h.drainDone

	recs := block.inner.snapshot()
	require.Equal(t, 2, len(recs))
	require.Equal(t, "queued", recs[0].Message)
	require.Equal(t, "sync", recs[1].Message)
}

// TestSyncEmitFlushesQueuedRecords proves SyncEmit drains the records already
// sitting in the queue before writing its own record, so the buffered context
// leading up to a fatal/panic survives a process exit that happens right after
// the call. The drain goroutine is stopped first (Close, then drainDone) so the
// flush runs in isolation with no concurrent consumer racing for the queue;
// records are staged directly because Handle no-ops post-Close.
func TestSyncEmitFlushesQueuedRecords(t *testing.T) {
	rec := &recordingHandler{}
	opts := DefaultOptions()
	opts.QueueSize = 8
	h, err := NewAsyncHandler(rec, opts)
	require.NoError(t, err)

	require.NoError(t, h.Close())
	<-h.drainDone
	require.Equal(t, 0, rec.count(), "no records should have been written before staging")

	h.queue <- queuedRecord{ctx: context.Background(), record: makeRecord(slog.LevelInfo, "q1")}
	h.queue <- queuedRecord{ctx: context.Background(), record: makeRecord(slog.LevelWarn, "q2")}

	require.NoError(t, h.SyncEmit(context.Background(), makeRecord(LevelFatal, "fatal")))

	recs := rec.snapshot()
	got := make([]string, len(recs))
	for i, r := range recs {
		got[i] = r.Message
	}
	require.Equal(t, []string{"q1", "q2", "fatal"}, got)
}

// TestAsyncHandlerCountsDrainErrors emits records into a handler that errors
// on every call and verifies the drain counts the errors and survives. The
// assertions run before Close so the close-flush summary doesn't perturb
// the error counter (the summary is itself sent through the downstream and
// would otherwise add one more error).
func TestAsyncHandlerCountsDrainErrors(t *testing.T) {
	errH := &erroringHandler{err: errors.New("boom")}
	opts := DefaultOptions()
	opts.SummaryInterval = time.Hour
	h, err := NewAsyncHandler(errH, opts)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		require.NoError(t, h.Handle(context.Background(), makeRecord(slog.LevelInfo, "x")))
	}

	require.Eventually(t, func() bool { return errH.count.Load() == 3 }, time.Second, time.Millisecond)
	require.Equal(t, int64(3), h.drainErrors.Load())

	require.NoError(t, h.Close())
	<-h.drainDone
}

func TestDropIdxBucketing(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  int
	}{
		{LevelTrace, 0},
		{LevelTrace - 1, 0},
		{slog.LevelDebug, 1},
		{slog.LevelDebug + 1, 1},
		{slog.LevelInfo, 2},
		{slog.LevelWarn, 3},
		{slog.LevelError, 4},
		{LevelFatal, 5},
		{LevelPanic, 6},
		{LevelPanic + 100, 6},
	}
	for _, tt := range tests {
		t.Run(LevelName(tt.level), func(t *testing.T) {
			require.Equal(t, tt.want, dropIdx(tt.level))
		})
	}
}
