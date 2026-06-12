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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogLevelStringRoundTrip(t *testing.T) {
	for _, lvl := range []LogLevel{PanicLevel, FatalLevel, ErrorLevel, WarnLevel, InfoLevel, DebugLevel, TraceLevel} {
		name := lvl.String()
		require.NotEqual(t, "unknown", name)
		parsed, err := ParseLogLevel(name)
		require.NoError(t, err)
		require.Equal(t, lvl, parsed)
	}
}

func TestLogLevelString(t *testing.T) {
	require.Equal(t, "info", InfoLevel.String())
	require.Equal(t, "trace", TraceLevel.String())
	require.Equal(t, "unknown", LogLevel(99).String())
}

func TestParseLogLevel(t *testing.T) {
	l, err := ParseLogLevel("DEBUG")
	require.NoError(t, err)
	require.Equal(t, DebugLevel, l)

	l, err = ParseLogLevel("warning")
	require.NoError(t, err)
	require.Equal(t, WarnLevel, l)

	_, err = ParseLogLevel("bogus")
	require.Error(t, err)
}
