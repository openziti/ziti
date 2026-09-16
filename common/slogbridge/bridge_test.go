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

package slogbridge

import (
	"context"
	"log"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/openziti/foundation/v2/logging"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
)

func newTestLogger() (*logrus.Logger, *test.Hook) {
	logger, hook := test.NewNullLogger()
	logger.SetLevel(logrus.TraceLevel)
	logger.SetReportCaller(true)
	return logger, hook
}

func Test_Bridge_LevelMapping(t *testing.T) {
	logger, hook := newTestLogger()
	log := slog.New(New(logger))

	cases := []struct {
		slogLevel   slog.Level
		logrusLevel logrus.Level
	}{
		{logging.LevelTrace, logrus.TraceLevel},
		{slog.LevelDebug, logrus.DebugLevel},
		{slog.LevelDebug + 1, logrus.DebugLevel},
		{slog.LevelInfo, logrus.InfoLevel},
		{slog.LevelWarn, logrus.WarnLevel},
		{slog.LevelError, logrus.ErrorLevel},
		{logging.LevelFatal, logrus.FatalLevel},
		{logging.LevelPanic, logrus.PanicLevel},
	}

	for _, c := range cases {
		hook.Reset()
		log.Log(context.Background(), c.slogLevel, "msg")
		entry := hook.LastEntry()
		require.NotNil(t, entry, "level %v produced no entry", c.slogLevel)
		require.Equal(t, c.logrusLevel, entry.Level, "level %v", c.slogLevel)
	}
}

// A fatal or panic record is emitted at its own severity, without the exit or panic that the
// logrus Fatal and Panic methods add, so it survives a logger whose threshold is fatal or panic.
func Test_Bridge_FatalAndPanicEmitAtOwnSeverity(t *testing.T) {
	logger, hook := newTestLogger()
	log := slog.New(New(logger))

	logger.SetLevel(logrus.FatalLevel)
	log.Log(context.Background(), slog.LevelError, "dropped at fatal threshold")
	require.Nil(t, hook.LastEntry())
	log.Log(context.Background(), logging.LevelFatal, "kept at fatal threshold")
	require.NotNil(t, hook.LastEntry())
	require.Equal(t, logrus.FatalLevel, hook.LastEntry().Level)

	hook.Reset()
	logger.SetLevel(logrus.PanicLevel)
	log.Log(context.Background(), logging.LevelFatal, "dropped at panic threshold")
	require.Nil(t, hook.LastEntry())
	require.NotPanics(t, func() {
		log.Log(context.Background(), logging.LevelPanic, "kept at panic threshold")
	})
	require.NotNil(t, hook.LastEntry())
	require.Equal(t, logrus.PanicLevel, hook.LastEntry().Level)
}

// Enabled must track the logrus level as it changes, since that is how the process level and the
// agent's set-log-level reach library output.
func Test_Bridge_EnabledFollowsLogrusLevel(t *testing.T) {
	logger, hook := newTestLogger()
	h := New(logger)
	log := slog.New(h)
	ctx := context.Background()

	logger.SetLevel(logrus.InfoLevel)
	require.False(t, h.Enabled(ctx, slog.LevelDebug))
	require.True(t, h.Enabled(ctx, slog.LevelInfo))
	log.Debug("dropped")
	require.Nil(t, hook.LastEntry())

	logger.SetLevel(logrus.DebugLevel)
	require.True(t, h.Enabled(ctx, slog.LevelDebug))
	log.Debug("kept")
	require.NotNil(t, hook.LastEntry())
	require.Equal(t, "kept", hook.LastEntry().Message)
}

func Test_Bridge_AttrsAndGroupsBecomeFields(t *testing.T) {
	logger, hook := newTestLogger()
	log := slog.New(New(logger)).With("service", "svc").WithGroup("conn").With("id", 7)

	log.Info("hello", "bytes", 12, slog.Group("peer", "addr", "1.2.3.4"), slog.Group("", "flat", true))

	entry := hook.LastEntry()
	require.NotNil(t, entry)
	require.Equal(t, "hello", entry.Message)
	require.Equal(t, "svc", entry.Data["service"])
	require.EqualValues(t, 7, entry.Data["conn.id"])
	require.EqualValues(t, 12, entry.Data["conn.bytes"])
	require.Equal(t, "1.2.3.4", entry.Data["conn.peer.addr"])
	require.Equal(t, true, entry.Data["conn.flat"], "an empty group name inlines its attrs")
}

// The entry's caller must be the function that logged, not the bridge, so pfxlog's function
// prefix stays meaningful for library output.
func Test_Bridge_CallerIsTheLoggingFunction(t *testing.T) {
	logger, hook := newTestLogger()
	log := slog.New(New(logger))

	log.Info("where am I")

	entry := hook.LastEntry()
	require.NotNil(t, entry)
	require.NotNil(t, entry.Caller)
	require.True(t, strings.HasSuffix(entry.Caller.Function, "Test_Bridge_CallerIsTheLoggingFunction"),
		"caller was %s", entry.Caller.Function)
}

func Test_Bridge_RecordTimeIsPreserved(t *testing.T) {
	logger, hook := newTestLogger()
	h := New(logger)

	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := slog.NewRecord(when, slog.LevelInfo, "at", 0)
	require.NoError(t, h.Handle(context.Background(), r))

	entry := hook.LastEntry()
	require.NotNil(t, entry)
	require.True(t, when.Equal(entry.Time))
	require.NotNil(t, entry.Caller)
	require.True(t, strings.HasSuffix(entry.Caller.Function, "(*Handler).Handle"),
		"a record without a PC falls back to logrus's own caller lookup, which lands on the bridge; got %s", entry.Caller.Function)
}

// Installing makes foundation's named loggers and the slog default land in logrus, filtered by
// the logrus level alone.
func Test_Bridge_InstallRoutesFoundationAndDefaultLoggers(t *testing.T) {
	logger, hook := newTestLogger()
	InstallTo(logger)

	logging.For("transport.test").Debug("from foundation", "endpoint", "tcp:127.0.0.1:1")
	entry := hook.LastEntry()
	require.NotNil(t, entry)
	require.Equal(t, logrus.DebugLevel, entry.Level)
	require.Equal(t, "from foundation", entry.Message)
	require.Equal(t, "tcp:127.0.0.1:1", entry.Data["endpoint"])

	hook.Reset()
	slog.Info("from default")
	require.NotNil(t, hook.LastEntry())
	require.Equal(t, "from default", hook.LastEntry().Message)

	hook.Reset()
	logger.SetLevel(logrus.WarnLevel)
	logging.For("transport.test").Info("filtered by logrus level")
	slog.Info("also filtered")
	require.Nil(t, hook.LastEntry())
	logging.For("transport.test").Warn("passes")
	require.NotNil(t, hook.LastEntry())
}

// The registry forwards records the logrus level will reject, so Handle must decline them before
// it allocates fields or symbolizes the caller.
func Test_Bridge_SuppressedRecordDoesNoWork(t *testing.T) {
	logger, hook := newTestLogger()
	logger.SetLevel(logrus.InfoLevel)
	h := New(logger).WithAttrs([]slog.Attr{slog.String("k", "v")})

	var pcs [1]uintptr
	runtime.Callers(1, pcs[:])
	r := slog.NewRecord(time.Now(), slog.LevelDebug, "suppressed", pcs[0])
	r.AddAttrs(slog.Int("n", 1))

	allocs := testing.AllocsPerRun(50, func() {
		_ = h.Handle(context.Background(), r)
	})
	require.Zero(t, allocs)
	require.Nil(t, hook.LastEntry())
}

// Output from the log package's default logger lands in logrus at info level and is attributed
// to the function that called log.Print, not to the bridge.
func Test_Bridge_StdlibLogIsAttributedToCaller(t *testing.T) {
	logger, hook := newTestLogger()
	InstallTo(logger)

	log.Print("via the log package")

	entry := hook.LastEntry()
	require.NotNil(t, entry)
	require.Equal(t, logrus.InfoLevel, entry.Level)
	require.Equal(t, "via the log package", entry.Message)
	require.NotNil(t, entry.Caller)
	require.True(t, strings.HasSuffix(entry.Caller.Function, "Test_Bridge_StdlibLogIsAttributedToCaller"),
		"caller was %s", entry.Caller.Function)

	hook.Reset()
	logger.SetLevel(logrus.WarnLevel)
	log.Print("filtered by logrus level")
	require.Nil(t, hook.LastEntry())
}
