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
	"io"
	"log/slog"
	"runtime"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestLevelMappingRoundTrip(t *testing.T) {
	cases := []struct {
		logrusLvl logrus.Level
		slogLvl   slog.Level
	}{
		{logrus.PanicLevel, LevelPanic},
		{logrus.FatalLevel, LevelFatal},
		{logrus.ErrorLevel, slog.LevelError},
		{logrus.WarnLevel, slog.LevelWarn},
		{logrus.InfoLevel, slog.LevelInfo},
		{logrus.DebugLevel, slog.LevelDebug},
		{logrus.TraceLevel, LevelTrace},
	}
	for _, c := range cases {
		require.Equal(t, c.slogLvl, logrusToSlog(c.logrusLvl), "logrusToSlog(%v)", c.logrusLvl)
		require.Equal(t, c.logrusLvl, slogToLogrus(c.slogLvl), "slogToLogrus(%v)", c.slogLvl)
	}
}

// TestSlogToLogrusBucketsNonCanonical proves non-canonical slog levels bucket
// into the canonical level whose value they most recently exceeded, matching
// the dropIdx bucketing used elsewhere in the package.
func TestSlogToLogrusBucketsNonCanonical(t *testing.T) {
	require.Equal(t, logrus.DebugLevel, slogToLogrus(slog.LevelDebug+1))
	require.Equal(t, logrus.ErrorLevel, slogToLogrus(slog.LevelError+1))
	require.Equal(t, logrus.PanicLevel, slogToLogrus(LevelPanic+100))
}

// TestBridgeFireInfoUsesAsyncQueue proves that a logrus Info entry routes
// through the async queue (not visible until the drain runs).
func TestBridgeFireInfoUsesAsyncQueue(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	async, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)

	Configure(async)
	SetGlobalLevel(LevelTrace)

	require.NoError(t, slogBridge{}.Fire(&logrus.Entry{
		Level:   logrus.InfoLevel,
		Time:    time.Now(),
		Message: "info-msg",
		Data:    logrus.Fields{"k": "v"},
	}))

	require.NoError(t, async.Close())
	<-async.drainDone

	require.Equal(t, 1, rec.count())
	rec0 := rec.snapshot()[0]
	require.Equal(t, "info-msg", rec0.Message)
	require.Equal(t, slog.LevelInfo, rec0.Level)

	attrs := map[string]any{}
	rec0.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	require.Equal(t, "v", attrs["k"])
}

// TestBridgeFireFatalIsSynchronous proves that a logrus Fatal entry routes
// through SyncEmit (synchronously through the downstream), so the record is
// durable before Fire returns. This is the durability guarantee for the
// logrus.Fatal path that calls os.Exit(1) immediately after hooks.
func TestBridgeFireFatalIsSynchronous(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	async, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)
	defer func() { _ = async.Close(); <-async.drainDone }()

	Configure(async)
	SetGlobalLevel(LevelTrace)

	require.NoError(t, slogBridge{}.Fire(&logrus.Entry{
		Level:   logrus.FatalLevel,
		Time:    time.Now(),
		Message: "fatal!",
	}))

	require.Equal(t, 1, rec.count(), "Fatal must reach downstream synchronously, before async drain")
	require.Equal(t, LevelFatal, rec.snapshot()[0].Level)
}

// TestBridgeFirePanicIsSynchronous mirrors the Fatal test for Panic.
func TestBridgeFirePanicIsSynchronous(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	async, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)
	defer func() { _ = async.Close(); <-async.drainDone }()

	Configure(async)
	SetGlobalLevel(LevelTrace)

	require.NoError(t, slogBridge{}.Fire(&logrus.Entry{
		Level:   logrus.PanicLevel,
		Time:    time.Now(),
		Message: "panic!",
	}))

	require.Equal(t, 1, rec.count(), "Panic must reach downstream synchronously")
	require.Equal(t, LevelPanic, rec.snapshot()[0].Level)
}

// TestInstallToWiresLogrus verifies the bridge invariant: after InstallTo,
// logrus output is io.Discard, the formatter is noopFormatter, the level is
// the mapped equivalent, and logrus log calls route through the bridge.
func TestInstallToWiresLogrus(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	async, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)

	target := logrus.New()
	InstallTo(target, async, slog.LevelInfo)

	require.Equal(t, io.Discard, target.Out)
	require.IsType(t, noopFormatter{}, target.Formatter)
	require.Equal(t, logrus.InfoLevel, target.Level)
	require.Equal(t, slog.LevelInfo, GlobalLevel(), "Install should set the global slog level too")

	target.WithField("k", "v").Info("hello")

	require.NoError(t, async.Close())
	<-async.drainDone

	require.Equal(t, 1, rec.count())
	require.Equal(t, "hello", rec.snapshot()[0].Message)
	require.Equal(t, slog.LevelInfo, rec.snapshot()[0].Level)
}

// TestInstallToFiltersBelowLevel proves logrus's own pre-filter still works
// after Install. Debug records below the configured level never reach the
// bridge, so they never reach the slog sink either.
func TestInstallToFiltersBelowLevel(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	async, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)

	target := logrus.New()
	InstallTo(target, async, slog.LevelInfo)

	target.Debug("filtered")
	target.Info("passed")

	require.NoError(t, async.Close())
	<-async.drainDone

	require.Equal(t, 1, rec.count())
	require.Equal(t, "passed", rec.snapshot()[0].Message)
}

// TestSyncEmitFallsBackForNonAsyncRoot proves SyncEmit handles a Registry
// whose root is not an *AsyncHandler by falling back to Handle. Handle on a
// non-async handler is already synchronous, so durability is preserved.
func TestSyncEmitFallsBackForNonAsyncRoot(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	Configure(rec) // root is the plain recording handler, not an AsyncHandler

	r := slog.NewRecord(time.Now(), LevelFatal, "fatal-fallback", 0)
	require.NoError(t, SyncEmit(context.Background(), r))
	require.Equal(t, 1, rec.count())
}

// TestSetGlobalLevelLocksLogrus proves the package-level SetGlobalLevel drives
// logrus.StandardLogger to the mapped equivalent, so logrus's own pre-filter
// stays consistent with the slog global. This is the entry point the agent
// callbacks use, and the lockstep is what keeps below-threshold records out
// of the bridge in the first place.
func TestSetGlobalLevelLocksLogrus(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	Configure(rec)

	prev := logrus.StandardLogger().Level
	t.Cleanup(func() { logrus.StandardLogger().SetLevel(prev) })

	SetGlobalLevel(slog.LevelInfo)
	require.Equal(t, logrus.InfoLevel, logrus.StandardLogger().Level)

	SetGlobalLevel(LevelTrace)
	require.Equal(t, logrus.TraceLevel, logrus.StandardLogger().Level)

	SetGlobalLevel(LevelFatal)
	require.Equal(t, logrus.FatalLevel, logrus.StandardLogger().Level)
}

// TestBridgeFireForwardsCallerPC proves the bridge populates the slog.Record's
// PC from logrus's entry.Caller. Downstream handlers that resolve the source
// frame from the PC (the pretty handler, sourceFlattener) see the original
// caller, not the bridge's own stack frame.
func TestBridgeFireForwardsCallerPC(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	async, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)

	Configure(async)
	SetGlobalLevel(LevelTrace)

	caller := &runtime.Frame{PC: 0xDEADBEEF, File: "x.go", Line: 7, Function: "x.Y"}
	require.NoError(t, slogBridge{}.Fire(&logrus.Entry{
		Level:   logrus.InfoLevel,
		Time:    time.Now(),
		Message: "with-caller",
		Caller:  caller,
	}))

	require.NoError(t, async.Close())
	<-async.drainDone

	require.Equal(t, 1, rec.count())
	require.Equal(t, uintptr(0xDEADBEEF), rec.snapshot()[0].PC, "bridge must forward entry.Caller.PC into slog.Record.PC")
}

// TestInstallToEnablesReportCaller proves InstallTo turns on ReportCaller so
// logrus populates entry.Caller and the bridge has a PC to forward.
func TestInstallToEnablesReportCaller(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	async, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)

	target := logrus.New()
	InstallTo(target, async, slog.LevelInfo)

	require.True(t, target.ReportCaller, "InstallTo must enable ReportCaller so the bridge gets a real PC")
}

// TestInstallToIsIdempotent proves repeated InstallTo on the same logger
// doesn't stack multiple slogBridge hooks. The quickstart hits this path by
// running the controller and router from a single process, each calling
// Install from their own PreRun: without idempotency, every bridged log line
// would dispatch twice through the bridge.
func TestInstallToIsIdempotent(t *testing.T) {
	resetDefaultForTest()
	rec := &recordingHandler{}
	async, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)

	target := logrus.New()
	InstallTo(target, async, slog.LevelInfo)
	InstallTo(target, async, slog.LevelInfo)
	target.Info("once")

	require.NoError(t, async.Close())
	<-async.drainDone
	require.Equal(t, 1, rec.count(), "repeated InstallTo must not stack bridges")
}
