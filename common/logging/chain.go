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
	"log/slog"
	"slices"
)

// boundHandler carries a set of attrs that get prepended to every record
// flowing through it, then delegates to its parent. WithAttrs never mutates
// the receiver; it returns a new boundHandler whose attrs are the receiver's
// combined with the additional ones and whose parent is the receiver's parent
// (not the receiver itself). The effect on a chain of slog.Logger.With calls
// is that they produce sibling boundHandlers at the same chain depth rather
// than stacking wrapper-on-wrapper, which would grow the chain on every call.
type boundHandler struct {
	parent slog.Handler
	attrs  []slog.Attr
}

func (h *boundHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.parent.Enabled(ctx, level)
}

func (h *boundHandler) Handle(ctx context.Context, r slog.Record) error {
	r2 := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r2.AddAttrs(h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		r2.AddAttrs(a)
		return true
	})
	return h.parent.Handle(ctx, r2)
}

func (h *boundHandler) WithAttrs(more []slog.Attr) slog.Handler {
	if len(more) == 0 {
		return h
	}
	combined := make([]slog.Attr, 0, len(h.attrs)+len(more))
	combined = append(combined, h.attrs...)
	combined = append(combined, more...)
	return &boundHandler{parent: h.parent, attrs: combined}
}

func (h *boundHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &groupedHandler{parent: h, name: name}
}

// groupedHandler wraps every record's attrs in slog.Group(name, ...) before
// delegating to its parent. A subsequent WithAttrs creates a boundHandler
// whose parent is this groupedHandler, so the attrs land inside the group.
type groupedHandler struct {
	parent slog.Handler
	name   string
}

func (h *groupedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.parent.Enabled(ctx, level)
}

func (h *groupedHandler) Handle(ctx context.Context, r slog.Record) error {
	var items []any
	r.Attrs(func(a slog.Attr) bool {
		items = append(items, a)
		return true
	})
	r2 := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r2.AddAttrs(slog.Group(h.name, items...))
	return h.parent.Handle(ctx, r2)
}

func (h *groupedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return &boundHandler{parent: h, attrs: slices.Clone(attrs)}
}

func (h *groupedHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &groupedHandler{parent: h, name: name}
}
