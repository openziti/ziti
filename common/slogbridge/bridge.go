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

// Package slogbridge routes log/slog records into logrus, so libraries that log through slog
// share the process's pfxlog output, formatting and level.
package slogbridge

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/openziti/foundation/v2/logging"
	"github.com/sirupsen/logrus"
)

// Handler is a slog.Handler that emits each record as a logrus entry at the corresponding
// logrus level. Emitting never exits or panics, even for fatal and panic records; that stays
// with the caller. Enabled consults the logrus logger's level on every call, so runtime level
// changes apply immediately. Handler is immutable; WithAttrs and WithGroup return copies. It
// is safe for concurrent use.
type Handler struct {
	logger *logrus.Logger
	fields logrus.Fields
	groups []string
}

var _ slog.Handler = (*Handler)(nil)

// New returns a Handler that writes to logger.
func New(logger *logrus.Logger) *Handler {
	return &Handler{logger: logger}
}

// Install routes slog output into the standard logrus logger: it makes a Handler the root of
// foundation's logging registry and the log/slog default, and opens the registry to every level
// so that the logrus level is the only filter. Call it once, after pfxlog is initialized.
func Install() {
	InstallTo(logrus.StandardLogger())
}

// InstallTo is Install for a specific logrus logger.
func InstallTo(logger *logrus.Logger) {
	h := New(logger)
	logging.Configure(h)
	logging.SetGlobalLevel(logging.LevelTrace)
	slog.SetDefault(slog.New(h))
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return h.logger.IsLevelEnabled(logrusLevel(level))
}

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	fields := make(logrus.Fields, len(h.fields)+r.NumAttrs())
	for k, v := range h.fields {
		fields[k] = v
	}
	prefix := h.prefix()
	r.Attrs(func(a slog.Attr) bool {
		addAttr(fields, prefix, a)
		return true
	})

	entry := logrus.NewEntry(h.logger).WithFields(fields)
	if !r.Time.IsZero() {
		entry.Time = r.Time
	} else {
		entry.Time = time.Now()
	}
	// logrus only resolves the caller itself when Caller is nil, so setting it here makes
	// pfxlog attribute the line to the library function that logged; a record without a PC
	// is left to logrus and comes out attributed to this handler
	if r.PC != 0 {
		frames := runtime.CallersFrames([]uintptr{r.PC})
		frame, _ := frames.Next()
		entry.Caller = &frame
	}
	// Entry.Log emits at any level without the exit or panic that Entry.Fatal and Entry.Panic add
	entry.Log(logrusLevel(r.Level), r.Message)
	return nil
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	fields := make(logrus.Fields, len(h.fields)+len(attrs))
	for k, v := range h.fields {
		fields[k] = v
	}
	prefix := h.prefix()
	for _, a := range attrs {
		addAttr(fields, prefix, a)
	}
	return &Handler{logger: h.logger, fields: fields, groups: h.groups}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	groups := make([]string, len(h.groups)+1)
	copy(groups, h.groups)
	groups[len(h.groups)] = name
	return &Handler{logger: h.logger, fields: h.fields, groups: groups}
}

func (h *Handler) prefix() string {
	if len(h.groups) == 0 {
		return ""
	}
	return strings.Join(h.groups, ".") + "."
}

// addAttr stores a as a field under prefix, flattening groups into dotted keys the way the
// stdlib text handler does. Empty attrs and empty-keyed groups are elided per the slog contract.
func addAttr(fields logrus.Fields, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		groupPrefix := prefix
		if a.Key != "" {
			groupPrefix = prefix + a.Key + "."
		}
		for _, ga := range a.Value.Group() {
			addAttr(fields, groupPrefix, ga)
		}
		return
	}
	fields[prefix+a.Key] = a.Value.Any()
}

// logrusLevel maps a slog level onto logrus's seven levels, treating foundation's trace, fatal and
// panic extensions as their logrus counterparts and non-canonical values by threshold.
func logrusLevel(l slog.Level) logrus.Level {
	switch {
	case l < slog.LevelDebug:
		return logrus.TraceLevel
	case l < slog.LevelInfo:
		return logrus.DebugLevel
	case l < slog.LevelWarn:
		return logrus.InfoLevel
	case l < slog.LevelError:
		return logrus.WarnLevel
	case l < logging.LevelFatal:
		return logrus.ErrorLevel
	case l < logging.LevelPanic:
		return logrus.FatalLevel
	default:
		return logrus.PanicLevel
	}
}
