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

package agent

import (
	"strings"

	"github.com/pkg/errors"
)

// LogLevel is the agent's transport-neutral log level. It is the type carried
// across the log-level callback boundary so that common/agent stays agnostic
// of any particular logging implementation (logrus, slog, ...); the embedding
// application maps LogLevel onto its own loggers.
type LogLevel int

const (
	PanicLevel LogLevel = iota
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
	TraceLevel
)

var logLevelNames = map[LogLevel]string{
	PanicLevel: "panic",
	FatalLevel: "fatal",
	ErrorLevel: "error",
	WarnLevel:  "warn",
	InfoLevel:  "info",
	DebugLevel: "debug",
	TraceLevel: "trace",
}

// String returns the canonical lowercase wire name for the level, or "unknown"
// for values outside the defined set.
func (l LogLevel) String() string {
	if name, ok := logLevelNames[l]; ok {
		return name
	}
	return "unknown"
}

// ParseLogLevel converts a canonical wire name (case-insensitive) into a
// LogLevel, returning an error for unrecognized names.
func ParseLogLevel(s string) (LogLevel, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "panic":
		return PanicLevel, nil
	case "fatal":
		return FatalLevel, nil
	case "error":
		return ErrorLevel, nil
	case "warn", "warning":
		return WarnLevel, nil
	case "info":
		return InfoLevel, nil
	case "debug":
		return DebugLevel, nil
	case "trace":
		return TraceLevel, nil
	default:
		return 0, errors.Errorf("invalid log level %q", s)
	}
}
