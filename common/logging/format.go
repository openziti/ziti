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
)

// ReplaceAttr is the slog.HandlerOptions.ReplaceAttr callback that coerces
// the standard slog JSON output into the pfxlog-shape downstream parsers
// already consume:
//
//   - the "level" key's value is the canonical lowercase name (via LevelName)
//     rather than slog's uppercase default;
//   - the nested "source" attr is suppressed entirely; sourceFlattener emits
//     "file" and "func" as flat top-level keys instead.
//
// Modifications apply only at the top level; attrs nested inside groups are
// passed through unchanged.
func ReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a
	}
	switch a.Key {
	case slog.LevelKey:
		if lvl, ok := a.Value.Any().(slog.Level); ok {
			return slog.String(slog.LevelKey, LevelName(lvl))
		}
	case slog.SourceKey:
		return slog.Attr{}
	}
	return a
}

// sourceFlattener decodes a record's PC and emits "file" and "func" as flat
// top-level attrs, matching pfxlog's existing JSON shape. The slog JSONHandler
// underneath has AddSource: false; this handler is the source of the file/func
// attrs in the output.
//
// Only records with a PC are decoded here. Direct slog records carry one, set
// by slog at the call site, and it re-decodes cleanly. Bridged-logrus records
// instead arrive with PC == 0 and their location already resolved into "file"
// and "func" attrs by the bridge (logrus's Entry.Caller.PC cannot be re-decoded
// reliably, so the bridge does not forward it); those records skip the decode
// here and pass their attrs through unchanged. Records built without any PC,
// like the AsyncHandler's drop-summary line, simply carry no file/func.
type sourceFlattener struct {
	parent slog.Handler
}

func (h *sourceFlattener) Enabled(ctx context.Context, level slog.Level) bool {
	return h.parent.Enabled(ctx, level)
}

func (h *sourceFlattener) Handle(ctx context.Context, r slog.Record) error {
	if r.PC == 0 {
		return h.parent.Handle(ctx, r)
	}
	frames := runtime.CallersFrames([]uintptr{r.PC})
	frame, _ := frames.Next()
	r2 := slog.NewRecord(r.Time, r.Level, r.Message, 0)
	r2.AddAttrs(slog.String("file", fmt.Sprintf("%s:%d", frame.File, frame.Line)))
	r2.AddAttrs(slog.String("func", frame.Function))
	r.Attrs(func(a slog.Attr) bool {
		r2.AddAttrs(a)
		return true
	})
	return h.parent.Handle(ctx, r2)
}

func (h *sourceFlattener) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &sourceFlattener{parent: h.parent.WithAttrs(attrs)}
}

func (h *sourceFlattener) WithGroup(name string) slog.Handler {
	return &sourceFlattener{parent: h.parent.WithGroup(name)}
}

// BuildHandler constructs the standard async slog handler chain that ziti
// uses in production: a JSON handler over out, wrapped in sourceFlattener
// for the pfxlog-shape file/func keys, wrapped in an AsyncHandler so writes
// happen off the caller's goroutine. The JSON handler itself does no level
// gating; gating lives upstream in the named-logger registry (Phase 5).
//
// out defaults to os.Stderr when nil.
func BuildHandler(out io.Writer, opts AsyncOptions) (*AsyncHandler, error) {
	if out == nil {
		out = os.Stderr
	}
	json := slog.NewJSONHandler(out, &slog.HandlerOptions{
		AddSource:   false,
		Level:       LevelTrace,
		ReplaceAttr: ReplaceAttr,
	})
	flat := &sourceFlattener{parent: json}
	return NewAsyncHandler(flat, opts)
}

// BuildTextHandler constructs the async chain for plain key=value text output:
// a slog TextHandler over out (lowercased canonical level names via
// ReplaceAttr, flat file/func via sourceFlattener), wrapped in an AsyncHandler.
// This is what --log-formatter=text selects, matching the pre-slog
// logrus.TextFormatter shape (level=info msg=...) rather than the colored,
// positional pretty output.
//
// out defaults to os.Stderr when nil.
func BuildTextHandler(out io.Writer, opts AsyncOptions) (*AsyncHandler, error) {
	if out == nil {
		out = os.Stderr
	}
	text := slog.NewTextHandler(out, &slog.HandlerOptions{
		AddSource:   false,
		Level:       LevelTrace,
		ReplaceAttr: ReplaceAttr,
	})
	flat := &sourceFlattener{parent: text}
	return NewAsyncHandler(flat, opts)
}

// BuildPrettyHandler builds the async chain for ziti's pretty (pfxlog-shape)
// output: a PrettyHandler over out, wrapped in an AsyncHandler. The
// PrettyHandler resolves the caller frame from slog.Record.PC, so the bridge
// must have ReportCaller enabled (which Install does) for legacy logrus call
// sites to render their original file/func.
//
// prettyOpts defaults to DefaultPrettyOptions(): pfxlog-compatible labels,
// "github.com/openziti/" trimmed from function names, relative time since the
// start of today, and color off unless PFXLOG_USE_COLOR opts in. out defaults
// to os.Stderr.
func BuildPrettyHandler(out io.Writer, opts AsyncOptions, prettyOpts *PrettyOptions) (*AsyncHandler, error) {
	if out == nil {
		out = os.Stderr
	}
	return NewAsyncHandler(NewPrettyHandler(out, prettyOpts), opts)
}

// Recognised values for the --log-formatter flag. "" is treated as
// FormatPretty so unconfigured binaries match the pre-slog default look.
const (
	FormatPretty = "pfxlog"
	FormatJSON   = "json"
	FormatText   = "text"
)

// BuildHandlerForFormat picks the production handler chain by name. Unknown
// or empty formats fall back to FormatPretty so ziti's default look-and-feel
// matches the pre-slog binaries.
func BuildHandlerForFormat(out io.Writer, opts AsyncOptions, format string) (*AsyncHandler, error) {
	switch format {
	case FormatJSON:
		return BuildHandler(out, opts)
	case FormatText:
		return BuildTextHandler(out, opts)
	case "", FormatPretty:
		return BuildPrettyHandler(out, opts, nil)
	default:
		return nil, fmt.Errorf("logging: unknown formatter %q (want %q, %q, or %q)", format, FormatPretty, FormatJSON, FormatText)
	}
}
