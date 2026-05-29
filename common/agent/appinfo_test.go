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
	"encoding/json"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadAppInfoV2ResponsePopulated(t *testing.T) {
	body, err := json.Marshal(AppInfoV2Response{
		Type:              "controller",
		AgentCapabilities: []string{"logging.slog-levels"},
		AppCapabilities:   []string{"ziti.foo"},
	})
	require.NoError(t, err)

	client, server := net.Pipe()
	go func() {
		_, _ = server.Write(body)
		_ = server.Close()
	}()

	resp, ok, err := ReadAppInfoV2Response(client)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "controller", resp.Type)
	require.Equal(t, []string{"logging.slog-levels"}, resp.AgentCapabilities)
	require.Equal(t, []string{"ziti.foo"}, resp.AppCapabilities)
}

// TestReadAppInfoV2ResponseCleanEOF proves an old server (no AppInfoV2 handler)
// that closes the connection with no write is detected as "unsupported".
func TestReadAppInfoV2ResponseCleanEOF(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		_ = server.Close()
	}()

	resp, ok, err := ReadAppInfoV2Response(client)
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, resp)
}

func TestCapabilitiesHelloRoundTrip(t *testing.T) {
	resetCaps()
	markAgentCapabilityActive(CapabilityLoggingSlogLevels)

	headers := map[int32][]byte{
		CapabilitiesHeader: GetAgentCapabilitiesMask().Bytes(),
	}
	mask := CapabilitiesFromHeaders(headers)

	names := GetAgentCapabilityStringList()
	require.NotEmpty(t, names)
	for _, name := range names {
		bit, ok := AgentCapabilityBitFromString(name)
		require.True(t, ok)
		require.Equal(t, uint(1), mask.Bit(bit))
	}

	require.Equal(t, 0, CapabilitiesFromHeaders(map[int32][]byte{}).BitLen())
}

func TestAgentCapabilityName(t *testing.T) {
	name, ok := AgentCapabilityName(CapabilityLoggingSlogLevels)
	require.True(t, ok)
	require.Equal(t, "logging.slog-levels", name)

	_, ok = AgentCapabilityName(99)
	require.False(t, ok)
}
