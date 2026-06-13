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
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLevelNameRoundTrip(t *testing.T) {
	for _, lvl := range []slog.Level{
		LevelTrace,
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
		LevelFatal,
		LevelPanic,
	} {
		name := LevelName(lvl)
		parsed, err := ParseLevel(name)
		require.NoError(t, err, "round-trip should succeed for %v", lvl)
		require.Equal(t, lvl, parsed)
	}
}

func TestLevelNameCanonical(t *testing.T) {
	require.Equal(t, "trace", LevelName(LevelTrace))
	require.Equal(t, "debug", LevelName(slog.LevelDebug))
	require.Equal(t, "info", LevelName(slog.LevelInfo))
	require.Equal(t, "warn", LevelName(slog.LevelWarn))
	require.Equal(t, "error", LevelName(slog.LevelError))
	require.Equal(t, "fatal", LevelName(LevelFatal))
	require.Equal(t, "panic", LevelName(LevelPanic))
}

// TestLevelNameOffsetFallback proves a non-canonical level value (e.g.
// slog.LevelDebug+1) emits valid lowercase output via slog.Level.String,
// rather than collapsing to an empty string or panicking. slog renders
// offsets relative to the nearest standard level, so the exact string depends
// on slog's own conventions; we just need it to be non-empty and lowercase.
func TestLevelNameOffsetFallback(t *testing.T) {
	require.Equal(t, "debug+1", LevelName(slog.LevelDebug+1))
	require.Equal(t, "error+1", LevelName(slog.LevelError+1))
}

func TestParseLevelAcceptsCaseAndWarningAlias(t *testing.T) {
	lvl, err := ParseLevel("DEBUG")
	require.NoError(t, err)
	require.Equal(t, slog.LevelDebug, lvl)

	lvl, err = ParseLevel("warning")
	require.NoError(t, err)
	require.Equal(t, slog.LevelWarn, lvl)

	lvl, err = ParseLevel("  Trace  ")
	require.NoError(t, err)
	require.Equal(t, LevelTrace, lvl)
}

func TestParseLevelRejectsUnknown(t *testing.T) {
	_, err := ParseLevel("bogus")
	require.Error(t, err)
}
