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

package handler_ctrl

import (
	"testing"

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/stretchr/testify/require"
)

// meshChannelType mirrors controller/raft/mesh.ChannelTypeMesh without importing that package.
const meshChannelType = "ctrl.mesh"

func helloWithType(chType string) *channel.Hello {
	h := channel.Headers{}
	if chType != "" {
		h.PutStringHeader(channel.TypeHeader, chType)
	}
	return &channel.Hello{Headers: h}
}

// Test_ConnectHandler_isSeparatelyValidated covers the rule that only channel types dispatched to a
// dedicated, self-validating acceptor are skipped; every other type - including unrecognized types and
// the mesh type on a non-clustered controller - routes to the router control acceptor and must be
// validated here.
func Test_ConnectHandler_isSeparatelyValidated(t *testing.T) {
	// Clustered controller: only the mesh type has a dedicated acceptor.
	clustered := &ConnectHandler{separatelyValidatedTypes: map[string]struct{}{meshChannelType: {}}}
	require.True(t, clustered.isSeparatelyValidated(helloWithType(meshChannelType)), "mesh is validated by its own acceptor")
	require.False(t, clustered.isSeparatelyValidated(helloWithType(ctrlchan.ChannelTypeDefault)), "router ctrl type is validated here")
	require.False(t, clustered.isSeparatelyValidated(helloWithType(ctrlchan.ChannelTypeHighPriority)))
	require.False(t, clustered.isSeparatelyValidated(helloWithType("bogus")), "unrecognized types route to the router acceptor and must be validated")
	require.False(t, clustered.isSeparatelyValidated(helloWithType("")), "legacy (no type) connections are validated here")

	// Non-clustered controller: no dedicated acceptors, so even a mesh-typed connection routes to the
	// router acceptor and must be validated.
	standalone := &ConnectHandler{separatelyValidatedTypes: map[string]struct{}{}}
	require.False(t, standalone.isSeparatelyValidated(helloWithType(meshChannelType)), "mesh on a non-clustered controller must be validated")
	require.False(t, standalone.isSeparatelyValidated(helloWithType(ctrlchan.ChannelTypeDefault)))
}

func Test_isFirstCtrlConnection(t *testing.T) {
	// Legacy / non-grouped dial: no grouped header -> treated as a new channel.
	require.True(t, isFirstCtrlConnection(&channel.Hello{Headers: channel.Headers{}}))

	grouped := channel.Headers{}
	grouped.PutBoolHeader(channel.IsGroupedHeader, true)
	grouped.PutBoolHeader(channel.IsFirstGroupConnection, true)
	require.True(t, isFirstCtrlConnection(&channel.Hello{Headers: grouped}), "grouped first connection")

	additional := channel.Headers{}
	additional.PutBoolHeader(channel.IsGroupedHeader, true)
	require.False(t, isFirstCtrlConnection(&channel.Hello{Headers: additional}), "additional underlay (no first flag)")

	notFirst := channel.Headers{}
	notFirst.PutBoolHeader(channel.IsGroupedHeader, true)
	notFirst.PutBoolHeader(channel.IsFirstGroupConnection, false)
	require.False(t, isFirstCtrlConnection(&channel.Hello{Headers: notFirst}), "additional underlay (first=false)")
}
