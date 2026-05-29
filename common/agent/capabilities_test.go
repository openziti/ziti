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

// resetCaps restores the package-level capability registry to its zero state
// so each test starts clean despite the registry being process-global.
func resetCaps() {
	capsMu.Lock()
	defer capsMu.Unlock()
	activeCaps = map[int]bool{}
	appCaps = nil
	capsFrozen = false
	logLevelCallbacks = nil
}

func TestAgentCapabilitiesInactiveByDefault(t *testing.T) {
	resetCaps()
	require.Empty(t, GetAgentCapabilityStringList())
	require.Equal(t, 0, GetAgentCapabilitiesMask().BitLen())
}

func TestAgentCapabilityActivation(t *testing.T) {
	resetCaps()
	markAgentCapabilityActive(CapabilityLoggingSlogLevels)
	require.Equal(t, []string{"logging.slog-levels"}, GetAgentCapabilityStringList())
	require.Equal(t, uint(1), GetAgentCapabilitiesMask().Bit(CapabilityLoggingSlogLevels))
}

func TestMaskMatchesStringList(t *testing.T) {
	resetCaps()
	markAgentCapabilityActive(CapabilityLoggingSlogLevels)
	mask := GetAgentCapabilitiesMask()
	names := GetAgentCapabilityStringList()
	require.NotEmpty(t, names)
	for _, name := range names {
		bit, ok := AgentCapabilityBitFromString(name)
		require.True(t, ok)
		require.Equal(t, uint(1), mask.Bit(bit))
	}
}

func TestAgentCapabilityBitFromString(t *testing.T) {
	bit, ok := AgentCapabilityBitFromString("logging.slog-levels")
	require.True(t, ok)
	require.Equal(t, CapabilityLoggingSlogLevels, bit)

	_, ok = AgentCapabilityBitFromString("nope")
	require.False(t, ok)
}

func TestRegisterAppCapabilitiesDedupOrder(t *testing.T) {
	resetCaps()
	RegisterAppCapabilities("ziti.foo", "ziti.foo", "ziti.bar")
	RegisterAppCapabilities("ziti.foo")
	require.Equal(t, []string{"ziti.foo", "ziti.bar"}, getAppCapabilities())
}

// TestAgentAndAppNamespacesIndependent proves the same string registered as
// both an agent capability and an app capability is reported on each list
// independently, with no merging.
func TestAgentAndAppNamespacesIndependent(t *testing.T) {
	resetCaps()
	markAgentCapabilityActive(CapabilityLoggingSlogLevels)
	RegisterAppCapabilities("logging.slog-levels")
	require.Equal(t, []string{"logging.slog-levels"}, GetAgentCapabilityStringList())
	require.Equal(t, []string{"logging.slog-levels"}, getAppCapabilities())
}

// TestRegisterAppCapabilitiesPanicsOnNewNameAfterFreeze proves a name that
// would add to the advertised list still panics post-freeze.
func TestRegisterAppCapabilitiesPanicsOnNewNameAfterFreeze(t *testing.T) {
	resetCaps()
	freezeCapabilities()
	require.Panics(t, func() {
		RegisterAppCapabilities("ziti.late")
	})
}

// TestRegisterAppCapabilitiesIdempotentAfterFreeze proves a name that is
// already in the advertised set passes through post-freeze without panic; the
// freeze rule only protects against set changes.
func TestRegisterAppCapabilitiesIdempotentAfterFreeze(t *testing.T) {
	resetCaps()
	RegisterAppCapabilities("ziti.foo", "ziti.bar")
	freezeCapabilities()
	require.NotPanics(t, func() {
		RegisterAppCapabilities("ziti.foo", "ziti.bar")
	})
	require.Equal(t, []string{"ziti.foo", "ziti.bar"}, getAppCapabilities())
}
