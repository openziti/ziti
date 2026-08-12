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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResolveFormatPrecedence pins the decision rules: an explicit flag always
// wins; with the flag unset, NO_JSON or a terminal selects pretty and a
// redirected (non-terminal) destination selects json.
func TestResolveFormatPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		flag       string
		noJson     bool
		isTerminal bool
		expected   string
	}{
		{"explicit json wins over tty", FormatJSON, false, true, FormatJSON},
		{"explicit json wins over NO_JSON", FormatJSON, true, false, FormatJSON},
		{"explicit pretty passthrough", FormatPretty, false, false, FormatPretty},
		{"explicit pfxlog alias passthrough", FormatPrettyAlias, false, false, FormatPrettyAlias},
		{"explicit text passthrough", FormatText, true, true, FormatText},
		{"unset + NO_JSON forces pretty", "", true, false, FormatPretty},
		{"unset + NO_JSON wins even on non-tty", "", true, false, FormatPretty},
		{"unset + terminal is pretty", "", false, true, FormatPretty},
		{"unset + redirected is json", "", false, false, FormatJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, resolveFormat(tt.flag, tt.noJson, tt.isTerminal))
		})
	}
}

// TestResolveFormatEnv proves ResolveFormat reads both the current and the
// deprecated NO_JSON variables, that an explicit flag still wins, and that a
// nil (non-terminal) destination with no env set defaults to json.
func TestResolveFormatEnv(t *testing.T) {
	t.Run("ZITI_LOG_NO_JSON forces pretty", func(t *testing.T) {
		t.Setenv(EnvNoJson, "true")
		require.Equal(t, FormatPretty, ResolveFormat("", nil))
	})

	t.Run("deprecated PFXLOG_NO_JSON still honored", func(t *testing.T) {
		t.Setenv(EnvNoJsonLegacy, "true")
		require.Equal(t, FormatPretty, ResolveFormat("", nil))
	})

	t.Run("explicit flag wins over env", func(t *testing.T) {
		t.Setenv(EnvNoJson, "true")
		require.Equal(t, FormatJSON, ResolveFormat(FormatJSON, nil))
	})

	t.Run("no env, redirected destination defaults to json", func(t *testing.T) {
		require.Equal(t, FormatJSON, ResolveFormat("", nil))
	})
}

// TestEnvBool covers the strconv.ParseBool-on-lowercased-value semantics
// inherited from pfxlog, including that an unparsable value is treated as
// false rather than panicking.
func TestEnvBool(t *testing.T) {
	const key = "ZITI_LOG_TEST_ENVBOOL"
	cases := map[string]bool{
		"true":  true,
		"TRUE":  true,
		"1":     true,
		"t":     true,
		"false": false,
		"0":     false,
		"nope":  false, // unparsable -> false
	}
	for value, expected := range cases {
		t.Run(value, func(t *testing.T) {
			t.Setenv(key, value)
			require.Equal(t, expected, envBool(key))
		})
	}

	t.Run("unset is false", func(t *testing.T) {
		require.NoError(t, os.Unsetenv(key))
		require.False(t, envBool(key))
	})
}
