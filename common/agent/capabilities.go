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
	"math/big"
	"slices"
	"sort"
	"sync"
)

// Agent capabilities. Each is owned by common/agent and has a stable bit
// position (used in the channel-hello bitmask) and a stable
// hierarchical-dotted string name (used in AppInfoV2.agent_capabilities). The
// two encodings stay in sync via agentCapabilityNames.
const (
	// CapabilityLoggingSlogLevels indicates the agent supports the channel-based v2
	// log-level commands (SetLogLevelV2, SetChannelLogLevelV2,
	// ClearChannelLogLevelV2), which carry string level names.
	CapabilityLoggingSlogLevels int = 1
)

// CapabilitiesHeader is the channel-hello header carrying the agent
// capability bitmask (the bytes of GetAgentCapabilitiesMask), in the
// agent-reserved header band.
const CapabilitiesHeader int32 = 30102

// agentCapabilityNames maps each agent capability bit to its canonical
// hierarchical-dotted string name. It is the single source of truth tying the
// bitmask encoding to the string-list encoding.
var agentCapabilityNames = map[int]string{
	CapabilityLoggingSlogLevels: "logging.slog-levels",
}

var (
	capsMu     sync.Mutex
	activeCaps = map[int]bool{} // agent caps whose handler is registered
	appCaps    []string         // app-registered capability strings, in registration order
	capsFrozen bool             // set once the agent listener starts
)

// markAgentCapabilityActive records that the handler backing an agent
// capability has been registered, so the capability becomes advertised. It is
// called by the registration entry points (e.g. RegisterLogLevelHandlers)
// before the listener starts.
func markAgentCapabilityActive(bit int) {
	capsMu.Lock()
	defer capsMu.Unlock()
	activeCaps[bit] = true
}

// GetAgentCapabilitiesMask returns the bitmask form of the active agent
// capabilities, for use in the channel hello.
func GetAgentCapabilitiesMask() *big.Int {
	capsMu.Lock()
	defer capsMu.Unlock()
	mask := &big.Int{}
	for bit := range activeCaps {
		mask.SetBit(mask, bit, 1)
	}
	return mask
}

// GetAgentCapabilityStringList returns the string-list form of the active agent
// capabilities, for use in AppInfoV2.agent_capabilities. Order is deterministic
// (sorted by bit position) so the JSON shape is stable.
func GetAgentCapabilityStringList() []string {
	capsMu.Lock()
	defer capsMu.Unlock()
	bits := make([]int, 0, len(activeCaps))
	for bit := range activeCaps {
		bits = append(bits, bit)
	}
	sort.Ints(bits)
	names := make([]string, 0, len(bits))
	for _, bit := range bits {
		if name, ok := agentCapabilityNames[bit]; ok {
			names = append(names, name)
		}
	}
	return names
}

// AgentCapabilityBitFromString is the inverse of agentCapabilityNames, used
// client-side to turn a name read from AppInfoV2 back into a bit. It consults
// the full registry, not just the active set.
func AgentCapabilityBitFromString(s string) (bit int, ok bool) {
	for b, name := range agentCapabilityNames {
		if name == s {
			return b, true
		}
	}
	return 0, false
}

// AgentCapabilityName returns the canonical string name for an agent capability
// bit, used client-side to check a bit constant against the AppInfoV2 string
// list.
func AgentCapabilityName(bit int) (string, bool) {
	name, ok := agentCapabilityNames[bit]
	return name, ok
}

// CapabilitiesFromHeaders decodes the agent capability bitmask from channel
// hello headers, returning an empty mask if the header is absent.
func CapabilitiesFromHeaders(headers map[int32][]byte) *big.Int {
	if val, ok := headers[CapabilitiesHeader]; ok {
		return new(big.Int).SetBytes(val)
	}
	return new(big.Int)
}

// RegisterAppCapabilities adds application-defined capability strings to the
// app_capabilities list emitted in AppInfoV2. The strings are passed through
// uninterpreted and deduplicated. Re-registering names that are already known
// is a no-op even after the listener has started, so multiple in-process apps
// can each declare the same capability without ordering concerns; introducing
// a new name after the listener starts panics, because that would drift the
// advertised set across connections.
func RegisterAppCapabilities(names ...string) {
	capsMu.Lock()
	defer capsMu.Unlock()
	for _, name := range names {
		// if we already have the capability, skip further processing
		if slices.Contains(appCaps, name) {
			continue
		}
		// First time we've seen this name, so registering it would change the
		// advertised set; that's only allowed before the listener starts.
		assertCapsMutable("RegisterAppCapabilities")
		appCaps = append(appCaps, name)
	}
}

// assertCapsMutable panics if the advertised capability set is sealed. Callers
// hold capsMu and only invoke this once they have determined the current call
// would actually change the set. The freeze rule exists so the bits and names
// reported in the channel hello stay consistent across every connection.
func assertCapsMutable(who string) {
	if capsFrozen {
		panic("agent: " + who + " would change the advertised capability set after the listener started")
	}
}

// agentCapAlreadyActive reports whether bit is already present in the
// advertised agent-capability set. A registration call that finds its bit
// already active is a callback/handler refresh, not a set change, so it is
// accepted post-freeze.
func agentCapAlreadyActive(bit int) bool {
	return activeCaps[bit]
}

// getAppCapabilities returns a copy of the registered app capability strings,
// in registration order.
func getAppCapabilities() []string {
	capsMu.Lock()
	defer capsMu.Unlock()
	return append([]string(nil), appCaps...)
}

// freezeCapabilities locks the capability set so no further registration is
// accepted. It is called when the agent listener starts.
func freezeCapabilities() {
	capsMu.Lock()
	defer capsMu.Unlock()
	capsFrozen = true
}
