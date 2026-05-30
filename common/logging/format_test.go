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
	"time"

	"github.com/stretchr/testify/require"
)

func TestReplaceAttrRenamesLevelLowercase(t *testing.T) {
	cases := []struct {
		lvl  slog.Level
		want string
	}{
		{LevelTrace, "trace"},
		{slog.LevelDebug, "debug"},
		{slog.LevelInfo, "info"},
		{slog.LevelWarn, "warn"},
		{slog.LevelError, "error"},
		{LevelFatal, "fatal"},
		{LevelPanic, "panic"},
	}
	for _, c := range cases {
		got := ReplaceAttr(nil, slog.Any(slog.LevelKey, c.lvl))
		require.Equal(t, slog.LevelKey, got.Key)
		require.Equal(t, c.want, got.Value.String())
	}
}

func TestReplaceAttrSuppressesSourceAtTopLevel(t *testing.T) {
	src := &slog.Source{Function: "f", File: "f.go", Line: 1}
	got := ReplaceAttr(nil, slog.Any(slog.SourceKey, src))
	require.Equal(t, slog.Attr{}, got, "source should be suppressed")
}

// TestReplaceAttrIgnoresInsideGroups proves the rename/suppression only
// applies to top-level attrs. A nested attr with key "level" (perhaps from
// caller code that happens to use that key inside a group) passes through
// unchanged.
func TestReplaceAttrIgnoresInsideGroups(t *testing.T) {
	a := slog.Any(slog.LevelKey, slog.LevelInfo)
	got := ReplaceAttr([]string{"g"}, a)
	require.Equal(t, a.Key, got.Key)
	require.Equal(t, a.Value, got.Value)
}

func TestReplaceAttrPassesThroughOtherAttrs(t *testing.T) {
	a := slog.String("k", "v")
	got := ReplaceAttr(nil, a)
	require.Equal(t, a, got)
}

// TestSourceFlattenerPassesThroughWhenPCZero proves records with no PC
// (e.g. those bridged from logrus, which sets file/func in Entry.Data) pass
// through the flattener unchanged.
func TestSourceFlattenerPassesThroughWhenPCZero(t *testing.T) {
	rec := &recordingHandler{}
	flat := &sourceFlattener{parent: rec}

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	r.AddAttrs(slog.String("k", "v"))
	require.NoError(t, flat.Handle(context.Background(), r))

	require.Equal(t, 1, rec.count())
	attrs := map[string]any{}
	rec.snapshot()[0].Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	require.NotContains(t, attrs, "file", "no file attr should be added when PC is zero")
	require.NotContains(t, attrs, "func")
	require.Equal(t, "v", attrs["k"])
}

// TestBuildHandlerEmitsPfxlogShape covers the BuildHandler end-to-end:
// asyncHandler -> sourceFlattener -> JSONHandler. A direct slog.Logger call
// produces JSON with lowercase level, flat file/func keys, no nested source
// object, and the caller's attrs preserved.
func TestBuildHandlerEmitsPfxlogShape(t *testing.T) {
	buf := &bytes.Buffer{}
	opts := DefaultOptions()
	opts.SummaryInterval = time.Hour
	h, err := BuildHandler(buf, opts)
	require.NoError(t, err)

	slog.New(h).Info("hello", "k", "v")

	require.NoError(t, h.Close())
	<-h.drainDone

	line := strings.TrimSpace(buf.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &got), "raw=%q", line)

	require.Equal(t, "hello", got["msg"])
	require.Equal(t, "info", got["level"], "level must be lowercase")
	require.Equal(t, "v", got["k"])

	require.NotContains(t, got, "source", "nested source must be suppressed")
	require.Contains(t, got, "file", "flat file key must be present")
	require.Contains(t, got, "func", "flat func key must be present")

	file, _ := got["file"].(string)
	require.Contains(t, file, ":", "file should be filename:line")
	require.NotEmpty(t, got["func"])
}

// TestBuildHandlerEmitsCustomLevels confirms LevelTrace, LevelFatal, and
// LevelPanic render with their canonical lowercase names rather than slog's
// offset form ("DEBUG-4" / "ERROR+4" / "ERROR+8").
func TestBuildHandlerEmitsCustomLevels(t *testing.T) {
	buf := &bytes.Buffer{}
	opts := DefaultOptions()
	opts.SummaryInterval = time.Hour
	h, err := BuildHandler(buf, opts)
	require.NoError(t, err)

	logger := slog.New(h)
	logger.Log(context.Background(), LevelTrace, "t")
	logger.Log(context.Background(), LevelFatal, "f")
	logger.Log(context.Background(), LevelPanic, "p")

	require.NoError(t, h.Close())
	<-h.drainDone

	var levels []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m))
		levels = append(levels, m["level"].(string))
	}
	require.Equal(t, []string{"trace", "fatal", "panic"}, levels)
}
