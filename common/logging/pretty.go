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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ANSI color sequences matching the mgutz/ansi values the old pfxlog
// formatter used, so colored output looks the same as the pre-slog binaries.
const (
	ansiRed        = "\033[31m"
	ansiYellow     = "\033[33m"
	ansiWhite      = "\033[37m"
	ansiBlue       = "\033[34m"
	ansiLightBlack = "\033[90m"
	ansiCyan       = "\033[36m"
	ansiLightCyan  = "\033[96m"
	ansiDefaultFg  = "\033[39m"
)

// Special attr keys carried over from pfxlog: ChannelsKey holds a []string of
// channel names rendered as |a, b| after the function, ContextKey holds a
// string rendered as [ctx]. Both are excluded from the fields block.
const (
	ChannelsKey = "_channels"
	ContextKey  = "_context"
)

// PrettyOptions configures PrettyHandler's human-readable output.
type PrettyOptions struct {
	// AbsoluteTime renders the record's time as a wall-clock timestamp using
	// TimestampFormat instead of seconds since StartTimestamp.
	AbsoluteTime bool

	// StartTimestamp is the baseline for the relative [seconds] time column.
	// DefaultPrettyOptions sets it to the start of today in local time,
	// matching pfxlog's StartingToday behavior.
	StartTimestamp time.Time

	// TimestampFormat is the time layout used when AbsoluteTime is set.
	TimestampFormat string

	// TrimPrefix is removed from the front of function names before they are
	// rendered. DefaultPrettyOptions sets "github.com/openziti/".
	TrimPrefix string

	// UseColor enables ANSI coloring of every colored segment (level label,
	// timestamp, function, fields). When false the output contains no escape
	// sequences at all.
	UseColor bool
}

// DefaultPrettyOptions returns PrettyOptions matching the pre-slog pfxlog
// defaults: relative time since the start of today, "github.com/openziti/"
// trimmed from function names, and color off unless PFXLOG_USE_COLOR opts in.
func DefaultPrettyOptions() *PrettyOptions {
	now := time.Now()
	return &PrettyOptions{
		StartTimestamp:  time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
		TimestampFormat: "2006-01-02 15:04:05.000",
		TrimPrefix:      "github.com/openziti/",
		UseColor:        useColor(),
	}
}

// useColor decides the default color setting. ziti has always run its pretty
// logs without color (cmd/main sets pfxlog's NoColor), so color is off unless a
// caller opts in via PFXLOG_USE_COLOR. TTY detection is deliberately not used:
// it would turn color on in interactive terminals where the pre-slog binaries
// showed none.
func useColor() bool {
	if env := os.Getenv("PFXLOG_USE_COLOR"); env != "" {
		if v, err := strconv.ParseBool(env); err == nil {
			return v
		}
	}
	return false
}

// PrettyHandler is a slog.Handler that renders records in the pfxlog pretty
// format the pre-slog ziti binaries produced:
//
//	[   12.345]   ERROR ziti/controller/server.Run: {k=[v]} something failed
//
// All seven canonical levels (trace through panic) render with their pfxlog
// labels; non-canonical levels fall back to slog's offset form. The handler
// does no level gating; that lives upstream in the registry chain.
type PrettyHandler struct {
	opts  PrettyOptions
	out   io.Writer
	lock  *sync.Mutex
	attrs []slog.Attr
}

// NewPrettyHandler builds a PrettyHandler writing to out. A nil opts uses
// DefaultPrettyOptions(); out defaults to os.Stderr when nil.
func NewPrettyHandler(out io.Writer, opts *PrettyOptions) *PrettyHandler {
	if out == nil {
		out = os.Stderr
	}
	if opts == nil {
		opts = DefaultPrettyOptions()
	}
	return &PrettyHandler{
		opts: *opts,
		out:  out,
		lock: &sync.Mutex{},
	}
}

func (h *PrettyHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	var out strings.Builder

	recordTime := r.Time
	if recordTime.IsZero() {
		recordTime = time.Now()
	}
	var timeLabel string
	if h.opts.AbsoluteTime {
		timeLabel = "[" + recordTime.Format(h.opts.TimestampFormat) + "]"
	} else {
		timeLabel = fmt.Sprintf("[%8.3f]", recordTime.Sub(h.opts.StartTimestamp).Seconds())
	}
	out.WriteString(h.colored(ansiBlue, timeLabel))

	out.WriteString(" " + h.levelLabel(r.Level))

	function := h.functionFor(r)

	// collect handler attrs then record attrs; later keys overwrite earlier
	// ones in the fields map, matching logrus WithField semantics
	fields := map[string]any{}
	addAttr := func(a slog.Attr) {
		fields[a.Key] = a.Value.Any()
	}
	for _, a := range h.attrs {
		addAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		addAttr(a)
		return true
	})

	// func/file attrs only stand in for the caller frame when there's no PC
	// (bridged records keep them in Entry.Data); with a PC they'd be
	// redundant with the resolved frame
	if function == "" {
		if fn, ok := fields["func"].(string); ok {
			function = fn
			delete(fields, "func")
			delete(fields, "file")
		}
	}
	function = strings.TrimPrefix(function, h.opts.TrimPrefix)

	if channels, ok := fields[ChannelsKey].([]string); ok && len(channels) > 0 {
		function += " |" + strings.Join(channels, ", ") + "|"
	}
	delete(fields, ChannelsKey)
	if logCtx, ok := fields[ContextKey].(string); ok {
		function += " [" + logCtx + "]"
	}
	delete(fields, ContextKey)

	out.WriteString(" " + h.colored(ansiCyan, function) + ":")

	if len(fields) > 0 {
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var fieldsStr strings.Builder
		fieldsStr.WriteString("{")
		for i, k := range keys {
			if i > 0 {
				fieldsStr.WriteString(" ")
			}
			fmt.Fprintf(&fieldsStr, "%s=[%v]", k, fields[k])
		}
		fieldsStr.WriteString("}")
		out.WriteString(" " + h.colored(ansiLightCyan, fieldsStr.String()))
	}

	out.WriteString(" " + r.Message)

	h.lock.Lock()
	defer h.lock.Unlock()
	_, err := fmt.Fprintln(h.out, out.String())
	return err
}

// WithAttrs returns a handler whose output includes attrs in the fields
// block of every record, appended after any attrs already held.
func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &PrettyHandler{opts: h.opts, out: h.out, lock: h.lock, attrs: merged}
}

// WithGroup returns the handler unchanged: the pfxlog pretty format has no
// group concept, so group qualification is intentionally dropped, as it was
// in the pfxlog and df handlers this replaces.
func (h *PrettyHandler) WithGroup(string) slog.Handler {
	return h
}

// functionFor resolves the record's caller function from its PC, or returns
// "" when there is no PC (bridged records carry func/file as attrs instead).
func (h *PrettyHandler) functionFor(r slog.Record) string {
	if r.PC == 0 {
		return ""
	}
	frames := runtime.CallersFrames([]uintptr{r.PC})
	frame, _ := frames.Next()
	return frame.Function
}

// levelLabel returns the 7-character pfxlog label for the level, colored when
// UseColor is set. Non-canonical levels render slog's offset form (for
// example "DEBUG+1") right-aligned to the same width.
func (h *PrettyHandler) levelLabel(l slog.Level) string {
	var label string
	switch l {
	case LevelPanic:
		label = "  PANIC"
	case LevelFatal:
		label = "  FATAL"
	case slog.LevelError:
		label = "  ERROR"
	case slog.LevelWarn:
		label = "WARNING"
	case slog.LevelInfo:
		label = "   INFO"
	case slog.LevelDebug:
		label = "  DEBUG"
	case LevelTrace:
		label = "  TRACE"
	default:
		label = fmt.Sprintf("%7s", l.String())
	}
	return h.colored(levelColor(l), label)
}

// levelColor buckets a level into the color of the canonical level at or
// below it, so non-canonical levels color like their nearest neighbor.
func levelColor(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return ansiRed
	case l >= slog.LevelWarn:
		return ansiYellow
	case l >= slog.LevelInfo:
		return ansiWhite
	case l >= slog.LevelDebug:
		return ansiBlue
	default:
		return ansiLightBlack
	}
}

// colored wraps s in the given color and a foreground reset when UseColor is
// set, and returns s unchanged otherwise.
func (h *PrettyHandler) colored(color, s string) string {
	if !h.opts.UseColor {
		return s
	}
	return color + s + ansiDefaultFg
}
