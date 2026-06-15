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

package agentlog

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common/agent"
	"github.com/openziti/ziti/v2/common/logging"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// recordingHandler captures every record that reaches it. The tests in this
// file install a sync recording handler (rather than the production
// AsyncHandler) so they don't depend on a queue drain to flush before
// assertions; this isolates the level-filtering behavior under test from the
// AsyncHandler's lifecycle.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.records))
	for i, r := range h.records {
		out[i] = r.Message
	}
	return out
}

// installForTest installs the sync recording handler onto logrus.StandardLogger
// (pfxlog v0.6.10 dispatches there, so the bridge must be wired to the
// standard logger for pfxlog calls to reach slog) and registers a cleanup
// that restores logrus's previous Out / Formatter / Level / ReportCaller and
// clears all hooks. Tests in this file mutate global logrus state by design.
func installForTest(t *testing.T, level slog.Level) *recordingHandler {
	t.Helper()
	rec := &recordingHandler{}

	std := logrus.StandardLogger()
	prevOut := std.Out
	prevFmt := std.Formatter
	prevLevel := std.Level
	prevReport := std.ReportCaller
	prevHooks := std.Hooks
	t.Cleanup(func() {
		std.SetOutput(prevOut)
		std.SetFormatter(prevFmt)
		std.SetLevel(prevLevel)
		std.SetReportCaller(prevReport)
		std.ReplaceHooks(prevHooks)
	})

	logging.Install(rec, level)
	return rec
}

// TestInstallInvariant proves logrus.StandardLogger is fully captured by the
// bridge after Install: output is io.Discard, formatter is the noop, the
// caller's level is mirrored to logrus, ReportCaller is on so the bridge has
// a PC to forward, and subsequent SetGlobalLevel updates keep the two worlds
// in lockstep. This is the invariant the Phase 6 design hangs on.
func TestInstallInvariant(t *testing.T) {
	installForTest(t, slog.LevelInfo)

	std := logrus.StandardLogger()
	require.Equal(t, io.Discard, std.Out, "output must be io.Discard")
	require.True(t, std.ReportCaller, "ReportCaller must be enabled")
	require.Equal(t, logrus.InfoLevel, std.Level, "logrus level must mirror the slog initial level")
	require.Equal(t, slog.LevelInfo, logging.GlobalLevel())

	// noopFormatter is unexported in common/logging; assert by behavior: the
	// formatter returns an empty byte slice for any entry, which is the
	// contract that makes the io.Discard sink benign.
	out, err := std.Formatter.Format(&logrus.Entry{Message: "x"})
	require.NoError(t, err)
	require.Empty(t, out)

	// Subsequent SetGlobalLevel must keep logrus pre-filter in lockstep.
	logging.SetGlobalLevel(logging.LevelTrace)
	require.Equal(t, logrus.TraceLevel, std.Level)
	logging.SetGlobalLevel(slog.LevelWarn)
	require.Equal(t, logrus.WarnLevel, std.Level)
}

// TestEndToEnd_AgentSetLogLevel proves the agent callback chain drives both
// sides of the bridge. After cb.SetLogLevel(InfoLevel), logrus's pre-filter
// drops Debug records (so they never reach the bridge) and the slog Registry
// reports the matching global; Info records flow through both the bridged
// logrus path (via pfxlog.Logger()) and the direct slog path (via
// logging.For()). This is the path `ziti agent set-log-level info` takes.
func TestEndToEnd_AgentSetLogLevel(t *testing.T) {
	rec := installForTest(t, slog.LevelWarn)
	cb := DefaultLogLevelCallbacks()

	// Initial level Warn: Info from either side is filtered.
	pfxlog.Logger().Info("pre-info-pfxlog")
	logging.For("zone.x").Info("pre-info-slog")
	require.Empty(t, rec.messages(), "pre-SetLogLevel Info records must be filtered")

	// Operator runs `ziti agent set-log-level info`; lockstep moves both sides.
	cb.SetLogLevel(agent.InfoLevel)
	require.Equal(t, slog.LevelInfo, logging.GlobalLevel())
	require.Equal(t, logrus.InfoLevel, logrus.StandardLogger().Level)

	// Below threshold remains filtered.
	pfxlog.Logger().Debug("post-debug-pfxlog")
	logging.For("zone.x").Debug("post-debug-slog")
	// At threshold: both paths emit.
	pfxlog.Logger().Info("post-info-pfxlog")
	logging.For("zone.x").Info("post-info-slog")

	got := rec.messages()
	require.Contains(t, got, "post-info-pfxlog", "bridged logrus Info must reach slog after SetLogLevel(Info)")
	require.Contains(t, got, "post-info-slog", "direct slog Info must reach the handler after SetLogLevel(Info)")
	require.NotContains(t, got, "post-debug-pfxlog", "logrus Debug must be filtered by the pre-filter after SetLogLevel(Info)")
	require.NotContains(t, got, "post-debug-slog", "slog Debug must be filtered by the global Registry level")
}

// TestPerChannelOverride_AppliesToSlogOnly_NotPfxlog proves Phase 7's slog-only
// channel-override semantics: when the global level is Info, an override
// lifting "test.gossip" to Debug enables slog Debug for that channel but does
// not affect pfxlog's channel-level mechanism. pfxlog.Logger() / ChannelLogger
// still observe the global logrus level (Info), so a pfxlog Debug call on the
// same channel name stays filtered. Clearing the override drops the channel
// back to the global level.
func TestPerChannelOverride_AppliesToSlogOnly_NotPfxlog(t *testing.T) {
	rec := installForTest(t, slog.LevelInfo)
	cb := DefaultLogLevelCallbacks()

	cb.SetChannelLogLevel("test.gossip", agent.DebugLevel)

	// slog override: Debug on the named channel is allowed through.
	logging.For("test.gossip").Debug("slog-debug-allowed")
	// pfxlog at Debug stays filtered because the global logrus level is Info
	// and the per-channel override does not touch pfxlog's overrides map.
	pfxlog.Logger().Debug("pfxlog-debug-filtered")
	pfxlog.ChannelLogger("test.gossip").Debug("pfxlog-channel-debug-filtered")
	// pfxlog at Info still gets through, confirming the bridge is wired.
	pfxlog.Logger().Info("pfxlog-info-allowed")

	got := rec.messages()
	require.Contains(t, got, "slog-debug-allowed", "slog channel override must enable Debug for that channel")
	require.Contains(t, got, "pfxlog-info-allowed", "bridge must continue to deliver pfxlog Info records")
	require.NotContains(t, got, "pfxlog-debug-filtered", "pfxlog Debug must stay filtered by the global logrus level")
	require.NotContains(t, got, "pfxlog-channel-debug-filtered", "pfxlog channel logger must not see the slog-only override")

	cb.ClearChannelLogLevel("test.gossip")
	logging.For("test.gossip").Debug("post-clear-debug-filtered")
	require.NotContains(t, rec.messages(), "post-clear-debug-filtered", "Clear must drop the channel back to the global Info level")
}
