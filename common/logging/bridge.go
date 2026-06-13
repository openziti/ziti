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
	"io"
	"log/slog"
	"os"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

// noopFormatter is the formatter logrus uses after Install. The actual
// dispatch happens in the slogBridge hook; the formatter's job is just to
// return an empty payload so logrus's own write (to io.Discard) does nothing.
type noopFormatter struct{}

func (noopFormatter) Format(*logrus.Entry) ([]byte, error) {
	return nil, nil
}

// slogBridge is the logrus.Hook that copies each logrus.Entry into a
// slog.Record and dispatches it onto the configured root handler.
// Fatal and Panic records bypass the async queue via SyncEmit so they are
// durable before logrus exits the process.
type slogBridge struct{}

func (slogBridge) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (slogBridge) Fire(e *logrus.Entry) error {
	level := logrusToSlog(e.Level)
	// logrus has already resolved the caller into e.Caller (Function, File,
	// Line). We must NOT forward e.Caller.PC and let a downstream handler
	// re-decode it: that PC is a symbolized runtime.Frame.PC, and re-running it
	// through runtime.CallersFrames resolves to the wrong function under inline
	// expansion (e.g. logrus.NewEntry instead of the real call site). Instead
	// the bridge carries the resolved location as flat file/func attrs and
	// leaves PC == 0, which is the contract sourceFlattener and PrettyHandler
	// expect for bridged records; direct slog records keep their PC and are
	// decoded by those handlers instead.
	r := slog.NewRecord(e.Time, level, e.Message, 0)
	if e.Caller != nil {
		r.AddAttrs(
			slog.String("file", fmt.Sprintf("%s:%d", e.Caller.File, e.Caller.Line)),
			slog.String("func", e.Caller.Function),
		)
	}
	for k, v := range e.Data {
		r.AddAttrs(slog.Any(k, v))
	}
	ctx := e.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if level >= LevelFatal {
		return SyncEmit(ctx, r)
	}
	return RootHandler().Handle(ctx, r)
}

// Install wires logrus.StandardLogger() into the async slog sink: it
// Configure's the default Registry with handler, sets the global slog level
// (which drives both slog and logrus in lockstep), redirects logrus output to
// io.Discard, replaces its formatter with a noop, enables ReportCaller so the
// bridge can forward a real PC to slog, and registers the slogBridge hook so
// every legacy log call routes through slog.
//
// Registering the hook replaces logrus's entire hook set, so any logrus hooks
// added before Install are discarded. ziti adds none; a caller that does must
// re-add them after Install.
//
// Calling Install more than once just replaces state; tests can call it
// freely. Anything that runs after Install must not later mutate logrus's
// output, formatter, or ReportCaller, or the bridge invariant breaks.
func Install(handler slog.Handler, initialLevel slog.Level) {
	InstallTo(logrus.StandardLogger(), handler, initialLevel)
}

// InstallTo is the testable form of Install: it wires the given *logrus.Logger
// instead of the global standard logger. Production code calls Install; tests
// pass logrus.New() so they can exercise the bridge without mutating global
// logrus state.
func InstallTo(target *logrus.Logger, handler slog.Handler, initialLevel slog.Level) {
	Configure(handler)
	target.SetOutput(io.Discard)
	target.SetFormatter(noopFormatter{})
	target.SetReportCaller(true)
	// Replace existing hooks before adding ours so repeated Install calls
	// don't stack multiple slogBridge instances on the same logger. The
	// quickstart exercises this by running the controller and router from
	// the same process, each through their own PreRun.
	target.ReplaceHooks(logrus.LevelHooks{})
	target.AddHook(slogBridge{})
	target.SetLevel(slogToLogrus(initialLevel))
	// SetGlobalLevel drives the lockstep slog + logrus.StandardLogger update.
	// Tests that pass a non-standard target via InstallTo already had the
	// target.SetLevel above; SetGlobalLevel still updates the standard logger
	// (a no-op when target is the standard logger), which is the production
	// path that the agent callbacks and CLI flags actually drive.
	SetGlobalLevel(initialLevel)
}

// RootHandler returns the default Registry's current root handler. The
// slogBridge dispatches records here; callers that need direct access
// (e.g. building a custom emitter) can use this too. Pre-Configure this
// returns the bootstrap stderr handler; post-Configure it returns
// whatever Install put in place.
func RootHandler() slog.Handler {
	return defaultRegistry.Root()
}

// SyncEmit writes r through the root handler synchronously on the caller's
// goroutine. If the root is an *AsyncHandler, the call routes through its
// SyncEmit, which flushes the queued records and then writes r under the same
// mutex the drain uses, so the buffered context survives a process exit right
// after. Otherwise, it falls back to Handle, which is already synchronous for
// any non-async handler.
//
// Used by the bridge for Fatal/Panic durability, and available to direct slog
// callers that need the same guarantee.
func SyncEmit(ctx context.Context, r slog.Record) error {
	root := RootHandler()
	if ah, ok := root.(*AsyncHandler); ok {
		return ah.SyncEmit(ctx, r)
	}
	return root.Handle(ctx, r)
}

// osExit is os.Exit, indirected so tests can exercise Fatal without
// terminating the test process.
var osExit = os.Exit

// Fatal writes msg at LevelFatal and then exits the process with status 1. It
// is the slog-world equivalent of logrus.Fatal, which slog does not provide
// (slog has only Debug/Info/Warn/Error and never exits the process itself).
//
// The record is emitted durably through SyncEmit, so it flushes any queued
// records and writes synchronously before the exit, and it bypasses level
// gating so a fatal is never filtered out. Hard-exit paths call this instead
// of logging an Error and then calling os.Exit or panic: those drop the record
// because the async queue never drains before the process is gone.
func Fatal(ctx context.Context, msg string, attrs ...slog.Attr) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip runtime.Callers + this frame
	r := slog.NewRecord(time.Now(), LevelFatal, msg, pcs[0])
	r.AddAttrs(attrs...)
	_ = SyncEmit(ctx, r)
	osExit(1)
}

// logrusToSlog maps a logrus.Level to its canonical slog.Level. logrus
// levels are densely packed (0..6, panic..trace); the seven canonical slog
// levels we use here map one-to-one.
func logrusToSlog(l logrus.Level) slog.Level {
	switch l {
	case logrus.TraceLevel:
		return LevelTrace
	case logrus.DebugLevel:
		return slog.LevelDebug
	case logrus.InfoLevel:
		return slog.LevelInfo
	case logrus.WarnLevel:
		return slog.LevelWarn
	case logrus.ErrorLevel:
		return slog.LevelError
	case logrus.FatalLevel:
		return LevelFatal
	case logrus.PanicLevel:
		return LevelPanic
	}
	return slog.LevelInfo
}

// slogToLogrus is the inverse of logrusToSlog for the seven canonical
// levels. Non-canonical slog levels (offsets between the canonical values)
// bucket into the canonical level whose value they most recently exceeded,
// matching the bucketing used by the drop counters.
func slogToLogrus(l slog.Level) logrus.Level {
	switch {
	case l < slog.LevelDebug:
		return logrus.TraceLevel
	case l < slog.LevelInfo:
		return logrus.DebugLevel
	case l < slog.LevelWarn:
		return logrus.InfoLevel
	case l < slog.LevelError:
		return logrus.WarnLevel
	case l < LevelFatal:
		return logrus.ErrorLevel
	case l < LevelPanic:
		return logrus.FatalLevel
	default:
		return logrus.PanicLevel
	}
}
