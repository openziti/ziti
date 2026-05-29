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

// Package logging is the async slog sink that ziti's logging refactor lands on.
// It owns the seven canonical log levels (trace through panic), the bounded
// AsyncHandler, and the SyncEmit path used for fatal/panic durability.
package logging

import (
	"log/slog"
	"strings"

	"github.com/pkg/errors"
)

// Custom slog.Level values extending slog's four standard levels (Debug=-4,
// Info=0, Warn=4, Error=8) with Trace below Debug and Fatal and Panic above
// Error. Together these cover the seven canonical level names ziti carries on
// the agent IPC wire.
const (
	LevelTrace slog.Level = -8
	LevelFatal slog.Level = 12
	LevelPanic slog.Level = 16
)

// LevelName returns the canonical lowercase name for a slog.Level on the wire.
// All seven canonical levels (trace, debug, info, warn, error, fatal, panic)
// map to their canonical names. Non-canonical values (e.g. slog.LevelDebug+1)
// fall back to a lowercased slog.Level.String(); slog renders those as
// "DEBUG+1" / "ERROR+4", so the wire never carries silent garbage.
func LevelName(l slog.Level) string {
	switch l {
	case LevelTrace:
		return "trace"
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	case LevelPanic:
		return "panic"
	}
	return strings.ToLower(l.String())
}

// ParseLevel converts a canonical level name (case-insensitive) into a
// slog.Level, returning an error for unrecognized names. Both "warn" and
// "warning" are accepted.
func ParseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "trace":
		return LevelTrace, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	case "fatal":
		return LevelFatal, nil
	case "panic":
		return LevelPanic, nil
	}
	return 0, errors.Errorf("invalid log level %q", name)
}
