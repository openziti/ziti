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

	"github.com/michaelquigley/df/dl"
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

// sourceFlattener decodes the record's PC and emits "file" and "func" as
// flat top-level attrs, matching pfxlog's existing JSON shape. The slog
// JSONHandler underneath has AddSource: false; this handler is the source of
// the file/func attrs in the output.
//
// Bridged-logrus records arrive with PC == 0 because logrus already supplies
// "file" and "func" via Entry.Data; the bridge copies those as attrs, so no
// PC decode is needed and the resulting record carries the same flat keys.
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

// BuildPrettyHandler builds the async chain for ziti's pretty (pfxlog-shape)
// output: a df/dl PrettyHandler over out, wrapped in an AsyncHandler. The
// PrettyHandler resolves the caller frame from slog.Record.PC, so the bridge
// must have ReportCaller enabled (which Install does) for legacy logrus call
// sites to render their original file/func.
//
// prettyOpts defaults to dl.DefaultOptions() (color when stdout is a TTY) but
// is always overridden to write to out so the caller controls the destination.
// out defaults to os.Stderr.
func BuildPrettyHandler(out io.Writer, opts AsyncOptions, prettyOpts *dl.Options) (*AsyncHandler, error) {
	if out == nil {
		out = os.Stderr
	}
	if prettyOpts == nil {
		prettyOpts = dl.DefaultOptions()
	}
	prettyOpts.Output = out
	prettyOpts.UseJSON = false
	return NewAsyncHandler(dl.NewPrettyHandler(LevelTrace, prettyOpts), opts)
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
		prettyOpts := dl.DefaultOptions()
		prettyOpts.UseColor = false
		return BuildPrettyHandler(out, opts, prettyOpts)
	case "", FormatPretty:
		return BuildPrettyHandler(out, opts, nil)
	default:
		return nil, fmt.Errorf("logging: unknown formatter %q (want %q, %q, or %q)", format, FormatPretty, FormatJSON, FormatText)
	}
}
