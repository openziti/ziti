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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestRegistry returns a Registry whose root is an AsyncHandler over the
// given downstream, along with the AsyncHandler so the caller can Close it
// and wait for drain.
func newTestRegistry(t *testing.T, downstream slog.Handler) (*Registry, *AsyncHandler) {
	t.Helper()
	h, err := NewAsyncHandler(downstream, DefaultOptions())
	require.NoError(t, err)
	return NewRegistry(h), h
}

func TestRegistryNewRejectsNilRoot(t *testing.T) {
	require.Panics(t, func() { NewRegistry(nil) })
}

func TestRegistryForPanicsOnEmpty(t *testing.T) {
	rec := &recordingHandler{}
	r, async := newTestRegistry(t, rec)
	defer func() { _ = async.Close(); <-async.drainDone }()
	require.Panics(t, func() { r.For("") })
	require.Panics(t, func() { r.HandlerFor("") })
}

func TestRegistryForCachesLoggers(t *testing.T) {
	rec := &recordingHandler{}
	r, async := newTestRegistry(t, rec)
	defer func() { _ = async.Close(); <-async.drainDone }()
	require.Same(t, r.For("x"), r.For("x"))
}

func TestRegistryGlobalLevelGates(t *testing.T) {
	rec := &recordingHandler{}
	r, async := newTestRegistry(t, rec)
	r.SetGlobalLevel(slog.LevelInfo)

	logger := r.For("x")
	logger.Debug("dropped-debug")
	logger.Info("kept-info")

	require.NoError(t, async.Close())
	<-async.drainDone

	var msgs []string
	for _, r := range rec.snapshot() {
		msgs = append(msgs, r.Message)
	}
	require.Equal(t, []string{"kept-info"}, msgs)
}

func TestRegistryNamedLevelOverridesGlobal(t *testing.T) {
	rec := &recordingHandler{}
	r, async := newTestRegistry(t, rec)
	r.SetGlobalLevel(slog.LevelInfo)
	r.SetNamedLevel("x", slog.LevelDebug)

	r.For("x").Debug("x-debug")
	r.For("y").Debug("y-debug-dropped")
	r.For("y").Info("y-info")

	require.NoError(t, async.Close())
	<-async.drainDone

	var msgs []string
	for _, r := range rec.snapshot() {
		msgs = append(msgs, r.Message)
	}
	require.ElementsMatch(t, []string{"x-debug", "y-info"}, msgs)
}

// TestRegistryClearNamedLevelRevertsToGlobal confirms ClearNamedLevel falls
// back to the live global, not a snapshot of it.
func TestRegistryClearNamedLevelRevertsToGlobal(t *testing.T) {
	rec := &recordingHandler{}
	r, async := newTestRegistry(t, rec)
	defer func() { _ = async.Close(); <-async.drainDone }()

	r.SetGlobalLevel(slog.LevelInfo)
	r.SetNamedLevel("x", slog.LevelDebug)
	require.True(t, r.For("x").Enabled(context.Background(), slog.LevelDebug))

	r.ClearNamedLevel("x")
	require.False(t, r.For("x").Enabled(context.Background(), slog.LevelDebug), "after clear, x should track the global")

	r.SetGlobalLevel(slog.LevelDebug)
	require.True(t, r.For("x").Enabled(context.Background(), slog.LevelDebug), "global change should reach the previously-overridden name")
}

// TestRegistryLevelChangesAffectExistingLoggers proves a Logger created
// before a level change reflects the new level on its next Enabled check.
func TestRegistryLevelChangesAffectExistingLoggers(t *testing.T) {
	rec := &recordingHandler{}
	r, async := newTestRegistry(t, rec)
	defer func() { _ = async.Close(); <-async.drainDone }()

	r.SetGlobalLevel(slog.LevelInfo)
	logger := r.For("x")
	require.False(t, logger.Enabled(context.Background(), slog.LevelDebug))

	r.SetGlobalLevel(slog.LevelDebug)
	require.True(t, logger.Enabled(context.Background(), slog.LevelDebug))
}

// TestRegistryComposedNamedLogger covers the design's acceptance gate:
//
//	For("router.link").WithGroup("g").With("k","v").Info("msg","x",1)
//	  -> {msg, channel:"router.link", g:{k:"v", x:1}}
func TestRegistryComposedNamedLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	downstream := slog.NewJSONHandler(buf, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey || a.Key == slog.LevelKey {
				return slog.Attr{}
			}
			return a
		},
	})
	r, async := newTestRegistry(t, downstream)
	r.For("router.link").WithGroup("g").With("k", "v").Info("msg", "x", 1)

	require.NoError(t, async.Close())
	<-async.drainDone

	line := strings.TrimSpace(buf.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &got), "raw=%q", line)
	require.Equal(t, "msg", got["msg"])
	require.Equal(t, "router.link", got["channel"])
	g, ok := got["g"].(map[string]any)
	require.True(t, ok, "g should be a nested object, got %T", got["g"])
	require.Equal(t, "v", g["k"])
	require.Equal(t, float64(1), g["x"])
}

// TestPackageDefaultWorksBeforeConfigure proves every package-level entry
// point is callable on the bootstrap Registry that init creates. Records
// emitted in the pre-Configure window are dropped silently by the discard
// root. This is the property that makes `var log = logging.For(...)` at
// package-init time safe.
func TestPackageDefaultWorksBeforeConfigure(t *testing.T) {
	resetDefaultForTest()
	require.NotPanics(t, func() { _ = For("x") })
	require.NotPanics(t, func() { SetGlobalLevel(slog.LevelInfo) })
	require.NotPanics(t, func() { SetNamedLevel("x", slog.LevelDebug) })
	require.NotPanics(t, func() { ClearNamedLevel("x") })
	require.NotPanics(t, func() { _ = GlobalLevel() })
	require.NotPanics(t, func() { _ = HandlerFor("x") })

	// A log call against For-logger goes through the bootstrap discard root
	// without error; the discard's Enabled returns false but namedHandler's
	// own gate is independent, so Handle is reached and silently drops the
	// record.
	require.NotPanics(t, func() { For("x").Info("dropped-silently") })
}

func TestPackageDefaultThroughConfigure(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	async, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)
	defer func() { _ = async.Close(); <-async.drainDone }()

	Configure(async)
	SetGlobalLevel(slog.LevelInfo)
	require.Equal(t, slog.LevelInfo, GlobalLevel())

	SetNamedLevel("x", slog.LevelDebug)
	require.True(t, For("x").Enabled(context.Background(), slog.LevelDebug))

	ClearNamedLevel("x")
	require.False(t, For("x").Enabled(context.Background(), slog.LevelDebug))
}

// TestConfigureSwapsRootInPlace proves a second Configure call replaces the
// default Registry's root handler without re-creating the Registry. Loggers
// captured before the swap stay valid: their next log call goes to the new
// root. This is the property that lets `var log = logging.For(...)`
// declarations land at package-init time and still work after Install
// runs from PreRun.
func TestConfigureSwapsRootInPlace(t *testing.T) {
	resetDefaultForTest()

	rec1 := &recordingHandler{}
	Configure(rec1)
	SetGlobalLevel(slog.LevelInfo)
	log := For("x")

	log.Info("first")
	require.Equal(t, 1, rec1.count(), "first record reaches the initial root")

	rec2 := &recordingHandler{}
	Configure(rec2)

	log.Info("second")
	require.Equal(t, 1, rec1.count(), "post-swap records must not reach the previous root")
	require.Equal(t, 1, rec2.count(), "post-swap records reach the new root via the same logger")

	require.Same(t, log, For("x"), "Configure swap must preserve the logger cache")
}

// TestRegistryConcurrentSetAndFor stress-tests concurrent For lookups and
// SetNamedLevel calls. Run under -race; the test passes if nothing panics
// and the race detector finds no data races.
func TestRegistryConcurrentSetAndFor(t *testing.T) {
	rec := &recordingHandler{}
	r, async := newTestRegistry(t, rec)
	defer func() { _ = async.Close(); <-async.drainDone }()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			r.For(fmt.Sprintf("logger-%d", i%4)).Info("x")
		}(i)
		go func(i int) {
			defer wg.Done()
			r.SetNamedLevel(fmt.Sprintf("logger-%d", i%4), slog.LevelDebug)
		}(i)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				r.ClearNamedLevel(fmt.Sprintf("logger-%d", i%4))
			} else {
				r.SetGlobalLevel(slog.LevelInfo)
			}
		}(i)
	}
	wg.Wait()
}

// discardHandler is used as the test-reset root: tests don't want the
// bootstrap stderr handler's output in their captured streams, and the
// rooted recording handlers tests typically install replace it anyway.
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (h discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h discardHandler) WithGroup(string) slog.Handler           { return h }

// resetDefaultForTest restores the package default Registry to a clean
// baseline: root reset to a silent discard (so test reset itself doesn't
// emit on stderr), overrides and logger cache cleared, global level
// reset to Info. Tests call this at start so they don't inherit state
// from earlier tests in the same binary. The Registry itself is the
// same instance throughout the test binary; only its internals are
// reset.
func resetDefaultForTest() {
	defaultRegistry.mu.Lock()
	defaultRegistry.overrides = map[string]*slog.LevelVar{}
	defaultRegistry.loggerCache = map[string]*slog.Logger{}
	defaultRegistry.mu.Unlock()
	defaultRegistry.SetRoot(discardHandler{})
	defaultRegistry.global.Set(slog.LevelInfo)
}
