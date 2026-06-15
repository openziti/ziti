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

// Package agentlog provides the ziti agent log-level callbacks. It is the one
// place that maps the agent's transport-neutral LogLevel onto ziti's loggers,
// so controller, router, and tunnel all register identical behavior.
package agentlog

import (
	"log/slog"

	"github.com/openziti/ziti/v2/common/agent"
	"github.com/openziti/ziti/v2/common/logging"
)

// DefaultLogLevelCallbacks returns the agent log-level callbacks that drive
// the common/logging registry. The global-level callback goes through
// logging.SetGlobalLevel, which moves slog and logrus.StandardLogger in
// lockstep; the channel callbacks drive logging.SetNamedLevel /
// logging.ClearNamedLevel, which apply slog-side only. Call sites that
// haven't migrated to logging.For(name) stay at the global level, which
// matches the design's migration carrot ("convert your package to slog to
// gain per-channel debug overrides").
func DefaultLogLevelCallbacks() agent.LogLevelCallbacks {
	return agent.LogLevelCallbacks{
		SetLogLevel: func(level agent.LogLevel) {
			logging.SetGlobalLevel(agentToSlog(level))
		},
		SetChannelLogLevel: func(channel string, level agent.LogLevel) {
			logging.SetNamedLevel(channel, agentToSlog(level))
		},
		ClearChannelLogLevel: func(channel string) {
			logging.ClearNamedLevel(channel)
		},
	}
}

// agentToSlog converts the agent's transport-neutral LogLevel into the
// equivalent slog.Level. The agent enum is finite and process-stable; an
// unrecognised value falls back to slog.LevelInfo so a future enum addition
// can't accidentally silence logging entirely.
func agentToSlog(level agent.LogLevel) slog.Level {
	switch level {
	case agent.PanicLevel:
		return logging.LevelPanic
	case agent.FatalLevel:
		return logging.LevelFatal
	case agent.ErrorLevel:
		return slog.LevelError
	case agent.WarnLevel:
		return slog.LevelWarn
	case agent.InfoLevel:
		return slog.LevelInfo
	case agent.DebugLevel:
		return slog.LevelDebug
	case agent.TraceLevel:
		return logging.LevelTrace
	default:
		return slog.LevelInfo
	}
}
