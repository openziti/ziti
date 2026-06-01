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
	"log/slog"
	"testing"

	"github.com/openziti/ziti/v2/common/agent"
	"github.com/openziti/ziti/v2/common/logging"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// TestAgentToSlogMapping covers the seven canonical levels by name so a
// future renumbering of either enum is caught here, not silently downstream.
func TestAgentToSlogMapping(t *testing.T) {
	cases := []struct {
		agentLvl agent.LogLevel
		slogLvl  slog.Level
	}{
		{agent.PanicLevel, logging.LevelPanic},
		{agent.FatalLevel, logging.LevelFatal},
		{agent.ErrorLevel, slog.LevelError},
		{agent.WarnLevel, slog.LevelWarn},
		{agent.InfoLevel, slog.LevelInfo},
		{agent.DebugLevel, slog.LevelDebug},
		{agent.TraceLevel, logging.LevelTrace},
	}
	for _, c := range cases {
		t.Run(c.agentLvl.String(), func(t *testing.T) {
			require.Equal(t, c.slogLvl, agentToSlog(c.agentLvl))
		})
	}
}

// TestAgentToSlogUnknownFallsBackToInfo proves an out-of-range LogLevel does
// not silence logging; it falls back to Info so log output is preserved.
func TestAgentToSlogUnknownFallsBackToInfo(t *testing.T) {
	require.Equal(t, slog.LevelInfo, agentToSlog(agent.LogLevel(99)))
}

// TestDefaultCallbacks_SetLogLevel proves SetLogLevel drives the slog global
// AND logrus.SetLevel via the package-level lockstep helper. This is the path
// `ziti agent set-log-level info` takes.
func TestDefaultCallbacks_SetLogLevel(t *testing.T) {
	prevLogrus := logrus.StandardLogger().Level
	t.Cleanup(func() { logrus.StandardLogger().SetLevel(prevLogrus) })

	configureForTest(t)
	cbs := DefaultLogLevelCallbacks()

	cbs.SetLogLevel(agent.DebugLevel)
	require.Equal(t, slog.LevelDebug, logging.GlobalLevel())
	require.Equal(t, logrus.DebugLevel, logrus.StandardLogger().Level)

	cbs.SetLogLevel(agent.WarnLevel)
	require.Equal(t, slog.LevelWarn, logging.GlobalLevel())
	require.Equal(t, logrus.WarnLevel, logrus.StandardLogger().Level)
}

// TestDefaultCallbacks_SetChannelLogLevel proves the channel callback drives
// the named-logger override; the slog Logger returned by logging.For honors
// the override on its next Enabled check.
func TestDefaultCallbacks_SetChannelLogLevel(t *testing.T) {
	configureForTest(t)
	cbs := DefaultLogLevelCallbacks()

	// Global Warn; channel "router.link" overridden to Debug.
	cbs.SetLogLevel(agent.WarnLevel)
	cbs.SetChannelLogLevel("router.link", agent.DebugLevel)

	logger := logging.For("router.link")
	require.True(t, logger.Enabled(context.Background(), slog.LevelDebug), "override must lift the channel above the global level")

	// Another channel without an override stays at global Warn.
	other := logging.For("router.xgress")
	require.False(t, other.Enabled(context.Background(), slog.LevelDebug), "non-overridden channels must follow the global level")
}

// TestDefaultCallbacks_ClearChannelLogLevel proves Clear restores the channel
// to the global level live; the channel logger's next Enabled check uses the
// global, even if it was below the previously-overridden level.
func TestDefaultCallbacks_ClearChannelLogLevel(t *testing.T) {
	configureForTest(t)
	cbs := DefaultLogLevelCallbacks()

	cbs.SetLogLevel(agent.WarnLevel)
	cbs.SetChannelLogLevel("router.link", agent.DebugLevel)
	cbs.ClearChannelLogLevel("router.link")

	logger := logging.For("router.link")
	require.False(t, logger.Enabled(context.Background(), slog.LevelDebug), "Clear must drop the channel back to the global level")
}

// configureForTest wires a no-op default Registry so the package-level
// logging functions don't panic. The Registry's root is a discard handler
// because tests here only assert level state, not record dispatch.
func configureForTest(t *testing.T) {
	t.Helper()
	logging.Configure(discardHandler{})
}

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (h discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h discardHandler) WithGroup(string) slog.Handler           { return h }
