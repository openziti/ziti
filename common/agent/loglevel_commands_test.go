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

func completeCallbacks() LogLevelCallbacks {
	return LogLevelCallbacks{
		SetLogLevel:          func(LogLevel) {},
		SetChannelLogLevel:   func(string, LogLevel) {},
		ClearChannelLogLevel: func(string) {},
	}
}

func TestRegisterLogLevelHandlersRequiresAllCallbacks(t *testing.T) {
	resetCaps()
	err := RegisterLogLevelHandlers(LogLevelCallbacks{})
	require.Error(t, err)
	require.Nil(t, getLogLevelCallbacks())
	require.Empty(t, GetAgentCapabilityStringList())
}

func TestRegisterLogLevelHandlersMarksCapabilityActive(t *testing.T) {
	resetCaps()
	require.NoError(t, RegisterLogLevelHandlers(completeCallbacks()))
	require.NotNil(t, getLogLevelCallbacks())
	require.Equal(t, []string{"logging.slog-levels"}, GetAgentCapabilityStringList())
	require.Equal(t, uint(1), GetAgentCapabilitiesMask().Bit(CapabilityLoggingSlogLevels))
}

// TestRegisterLogLevelHandlersPanicsOnFirstRegistrationAfterFreeze proves the
// freeze rule still bites when a call would actually add the capability bit.
func TestRegisterLogLevelHandlersPanicsOnFirstRegistrationAfterFreeze(t *testing.T) {
	resetCaps()
	freezeCapabilities()
	require.Panics(t, func() {
		_ = RegisterLogLevelHandlers(completeCallbacks())
	})
}

// TestRegisterLogLevelHandlersIdempotentAfterFreeze proves a second call after
// freeze succeeds when the capability bit is already active. The advertised
// set doesn't change, so only the callbacks get updated. This is the path
// taken in the quickstart, where the controller registers before Listen
// freezes, and the router registers again afterwards.
func TestRegisterLogLevelHandlersIdempotentAfterFreeze(t *testing.T) {
	resetCaps()
	require.NoError(t, RegisterLogLevelHandlers(completeCallbacks()))
	freezeCapabilities()

	calls := 0
	updated := LogLevelCallbacks{
		SetLogLevel:          func(LogLevel) { calls++ },
		SetChannelLogLevel:   func(string, LogLevel) {},
		ClearChannelLogLevel: func(string) {},
	}
	require.NotPanics(t, func() {
		require.NoError(t, RegisterLogLevelHandlers(updated))
	})

	cbs := getLogLevelCallbacks()
	require.NotNil(t, cbs)
	cbs.SetLogLevel(InfoLevel)
	require.Equal(t, 1, calls, "second registration must replace the callbacks in place")
}
