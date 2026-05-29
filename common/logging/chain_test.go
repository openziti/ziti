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
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// runWithLogger builds a fresh AsyncHandler over a JSON downstream, lets the
// caller emit one record through a slog.Logger backed by the handler, then
// closes the handler, waits for the drain, and returns the parsed JSON. Time
// and level keys are filtered out so tests can assert on shape alone.
func runWithLogger(t *testing.T, f func(*slog.Logger)) map[string]any {
	t.Helper()
	buf := &bytes.Buffer{}
	downstream := slog.NewJSONHandler(buf, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey || a.Key == slog.LevelKey {
				return slog.Attr{}
			}
			return a
		},
	})
	h, err := NewAsyncHandler(downstream, DefaultOptions())
	require.NoError(t, err)

	f(slog.New(h))

	require.NoError(t, h.Close())
	<-h.drainDone

	line := strings.TrimSpace(buf.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &got), "raw=%q", line)
	return got
}

func TestChainWithAttrs(t *testing.T) {
	got := runWithLogger(t, func(l *slog.Logger) {
		l.With("a", 1).Info("msg", "b", 2)
	})
	require.Equal(t, "msg", got["msg"])
	require.Equal(t, float64(1), got["a"])
	require.Equal(t, float64(2), got["b"])
}

func TestChainWithGroup(t *testing.T) {
	got := runWithLogger(t, func(l *slog.Logger) {
		l.WithGroup("g").Info("msg", "a", 1)
	})
	require.Equal(t, "msg", got["msg"])
	g, ok := got["g"].(map[string]any)
	require.True(t, ok, "g should be a nested object, got %T", got["g"])
	require.Equal(t, float64(1), g["a"])
}

// TestChainBoundAttrsBeforeGroup: With("a",1).WithGroup("g").With("b",2)
//
//	.Info("msg","c",3)  ->  {msg, a:1, g:{b:2, c:3}}
func TestChainBoundAttrsBeforeGroup(t *testing.T) {
	got := runWithLogger(t, func(l *slog.Logger) {
		l.With("a", 1).WithGroup("g").With("b", 2).Info("msg", "c", 3)
	})
	require.Equal(t, "msg", got["msg"])
	require.Equal(t, float64(1), got["a"])
	g, ok := got["g"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(2), g["b"])
	require.Equal(t, float64(3), g["c"])
}

// TestChainAttrsInsideGroup: WithGroup("g").With("a",1).Info("msg","b",2)
//
//	->  {msg, g:{a:1, b:2}}  (the .With after WithGroup lands inside the group)
func TestChainAttrsInsideGroup(t *testing.T) {
	got := runWithLogger(t, func(l *slog.Logger) {
		l.WithGroup("g").With("a", 1).Info("msg", "b", 2)
	})
	require.Equal(t, "msg", got["msg"])
	g, ok := got["g"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(1), g["a"])
	require.Equal(t, float64(2), g["b"])
	require.NotContains(t, got, "a", "a should be inside g, not at the root")
}

// TestChainNestedGroups: WithGroup("a").WithGroup("b").Info("msg","c",3)
//
//	->  {msg, a:{b:{c:3}}}
func TestChainNestedGroups(t *testing.T) {
	got := runWithLogger(t, func(l *slog.Logger) {
		l.WithGroup("a").WithGroup("b").Info("msg", "c", 3)
	})
	require.Equal(t, "msg", got["msg"])
	a, ok := got["a"].(map[string]any)
	require.True(t, ok)
	b, ok := a["b"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(3), b["c"])
}

// TestChainWithAttrsDoesNotNestWrappers proves that two consecutive WithAttrs
// calls on AsyncHandler -> boundHandler produce a single boundHandler whose
// parent is still AsyncHandler (a sibling at the same chain depth), not a
// new boundHandler wrapping the first one. This keeps chain depth bounded
// when slog.Logger.With is called repeatedly.
func TestChainWithAttrsDoesNotNestWrappers(t *testing.T) {
	rec := &recordingHandler{}
	h, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)
	defer func() { _ = h.Close(); <-h.drainDone }()

	first := h.WithAttrs([]slog.Attr{slog.Int("a", 1)})
	second := first.WithAttrs([]slog.Attr{slog.Int("b", 2)})

	bh, ok := second.(*boundHandler)
	require.True(t, ok, "expected *boundHandler, got %T", second)
	require.Equal(t, h, bh.parent, "parent should still be AsyncHandler, not the first boundHandler")
	require.Len(t, bh.attrs, 2)
	require.Equal(t, "a", bh.attrs[0].Key)
	require.Equal(t, "b", bh.attrs[1].Key)
}

// TestChainWithGroupEmptyIsNoop proves WithGroup("") returns the receiver
// unchanged on each of the three handler types.
func TestChainWithGroupEmptyIsNoop(t *testing.T) {
	rec := &recordingHandler{}
	h, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)
	defer func() { _ = h.Close(); <-h.drainDone }()

	require.Same(t, h, h.WithGroup(""), "AsyncHandler.WithGroup(\"\") should return receiver")

	bh := h.WithAttrs([]slog.Attr{slog.Int("a", 1)}).(*boundHandler)
	require.Same(t, bh, bh.WithGroup("").(*boundHandler), "boundHandler.WithGroup(\"\") should return receiver")

	gh := h.WithGroup("g").(*groupedHandler)
	require.Same(t, gh, gh.WithGroup("").(*groupedHandler), "groupedHandler.WithGroup(\"\") should return receiver")
}

// TestChainWithAttrsEmptyIsNoop proves WithAttrs(nil) returns the receiver
// on each of the three handler types, so slog.Logger.With() with no args
// allocates nothing.
func TestChainWithAttrsEmptyIsNoop(t *testing.T) {
	rec := &recordingHandler{}
	h, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)
	defer func() { _ = h.Close(); <-h.drainDone }()

	require.Same(t, h, h.WithAttrs(nil))

	bh := h.WithAttrs([]slog.Attr{slog.Int("a", 1)}).(*boundHandler)
	require.Same(t, bh, bh.WithAttrs(nil).(*boundHandler))

	gh := h.WithGroup("g").(*groupedHandler)
	require.Same(t, gh, gh.WithAttrs(nil).(*groupedHandler))
}

// TestChainSiblingLoggersDoNotLeakAttrs proves that deriving a child logger
// inside a loop and reusing the parent logger after the loop is safe: the
// parent's attrs are not contaminated by the children's additions. This is a
// common pattern (a parent logger plus per-iteration enrichment) and the
// extends-in-place optimization in boundHandler.WithAttrs must not break it.
func TestChainSiblingLoggersDoNotLeakAttrs(t *testing.T) {
	buf := &bytes.Buffer{}
	downstream := slog.NewJSONHandler(buf, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey || a.Key == slog.LevelKey {
				return slog.Attr{}
			}
			return a
		},
	})
	h, err := NewAsyncHandler(downstream, DefaultOptions())
	require.NoError(t, err)

	parent := slog.New(h).With("scope", "outer")
	parent.Info("before-loop")
	for _, v := range []int{1, 2, 3} {
		parent.With("v", v).Info("in-loop")
	}
	parent.Info("after-loop")

	require.NoError(t, h.Close())
	<-h.drainDone

	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &m))
		lines = append(lines, m)
	}
	require.Len(t, lines, 5)

	require.Equal(t, "before-loop", lines[0]["msg"])
	require.Equal(t, "outer", lines[0]["scope"])
	require.NotContains(t, lines[0], "v")

	for i := 1; i <= 3; i++ {
		require.Equal(t, "in-loop", lines[i]["msg"])
		require.Equal(t, "outer", lines[i]["scope"])
		require.Equal(t, float64(i), lines[i]["v"])
	}

	require.Equal(t, "after-loop", lines[4]["msg"])
	require.Equal(t, "outer", lines[4]["scope"])
	require.NotContains(t, lines[4], "v", "parent logger must not have inherited a child's attr")
}

// TestChainEnabledDelegates proves Enabled on chain wrappers reaches
// AsyncHandler, which currently returns true.
func TestChainEnabledDelegates(t *testing.T) {
	rec := &recordingHandler{}
	h, err := NewAsyncHandler(rec, DefaultOptions())
	require.NoError(t, err)
	defer func() { _ = h.Close(); <-h.drainDone }()

	bh := h.WithAttrs([]slog.Attr{slog.Int("a", 1)})
	gh := h.WithGroup("g")

	require.True(t, bh.Enabled(context.Background(), slog.LevelDebug))
	require.True(t, gh.Enabled(context.Background(), slog.LevelDebug))
}
