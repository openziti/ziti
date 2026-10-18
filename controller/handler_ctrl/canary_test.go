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

	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Test_canaryHandler_decodeCanary_carriesTheRoutersReport covers the router's version and capabilities, which the leader
// persists from the gossiped value, so they have to survive the trip from the canary payload.
func Test_canaryHandler_decodeCanary_carriesTheRoutersReport(t *testing.T) {
	caps := capabilities.NewMask(capabilities.RouterLinkGossip)
	body, err := proto.Marshal(&gossip_pb.CanaryPayload{
		EntryCounts:  map[string]int64{"links": 3},
		SentAt:       42,
		Version:      "v2.1.0",
		Capabilities: caps.Bytes(),
	})
	require.NoError(t, err)

	msg := channel.NewMessage(int32(ctrl_pb.ContentType_CanaryType), body)
	msg.PutUint64Header(int32(ctrl_pb.ControlHeaders_CanarySeqHeader), 7)
	msg.Headers[int32(ctrl_pb.ControlHeaders_EpochHeader)] = []byte{1, 2}

	value, sentAt, ok := (&canaryHandler{}).decodeCanary(msg)
	require.True(t, ok)
	require.Equal(t, uint64(7), value.Seq)
	require.Equal(t, []byte{1, 2}, value.Epoch)
	require.Equal(t, map[string]int64{"links": 3}, value.EntryCounts)
	require.Equal(t, "v2.1.0", value.Version)
	require.Equal(t, caps.Bytes(), value.Capabilities)
	require.Equal(t, int64(42), sentAt)
}

func Test_canaryHandler_decodeCanary_requiresASequence(t *testing.T) {
	_, _, ok := (&canaryHandler{}).decodeCanary(channel.NewMessage(int32(ctrl_pb.ContentType_CanaryType), nil))
	require.False(t, ok)
}
