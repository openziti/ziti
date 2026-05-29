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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common/agent"
	"github.com/sirupsen/logrus"
)

// DefaultLogLevelCallbacks returns the agent log-level callbacks that drive
// logrus (global level) and pfxlog (per-channel overrides), matching the
// behavior of the legacy framed log-level handlers. The agent LogLevel enum is
// ordered to match logrus.Level, so the conversion is a direct cast.
func DefaultLogLevelCallbacks() agent.LogLevelCallbacks {
	return agent.LogLevelCallbacks{
		SetLogLevel: func(level agent.LogLevel) {
			logrus.SetLevel(logrus.Level(level))
		},
		SetChannelLogLevel: func(channel string, level agent.LogLevel) {
			pfxlog.GlobalConfig(func(options *pfxlog.Options) *pfxlog.Options {
				options.SetChannelLogLevel(channel, logrus.Level(level))
				return options
			})
		},
		ClearChannelLogLevel: func(channel string) {
			pfxlog.GlobalConfig(func(options *pfxlog.Options) *pfxlog.Options {
				options.ClearChannelLogLevel(channel)
				return options
			})
		},
	}
}
