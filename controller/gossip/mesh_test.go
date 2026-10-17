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

package gossip

import (
	"testing"

	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/stretchr/testify/require"
)

// Test_cloneForPeer_independentHeaders guards the broadcast race fix: each peer
// must get a message with its own Headers map. A peer channel writes a header on
// its own tx goroutine (the heartbeater transform handler), so if peers shared a
// message, one peer's header write would race another peer's marshal iteration of
// the same map -> the fatal "concurrent map iteration and map write". The clone
// must therefore have a distinct Headers map while sharing the (read-only) body.
func Test_cloneForPeer_independentHeaders(t *testing.T) {
	req := require.New(t)

	body := []byte("gossip-delta-body")
	orig := channel.NewMessage(gossip_pb.GossipDeltaType, body)
	orig.Headers[1] = []byte("a")

	clone := cloneForPeer(orig)

	req.NotSame(orig, clone, "clone must be a distinct message instance")
	req.Equal(orig.ContentType, clone.ContentType)
	req.Equal(orig.Headers, clone.Headers, "existing headers are copied")

	// The body is shared (read-only during marshal); same backing array is fine.
	req.Equal(orig.Body, clone.Body)

	// Mutating the clone's headers (as a peer channel's tx handler does) must not
	// touch the original's map, and vice versa.
	clone.Headers[2] = []byte("b")
	_, origHasPeerWrite := orig.Headers[2]
	req.False(origHasPeerWrite, "writing the clone's headers must not mutate the original")

	orig.Headers[3] = []byte("c")
	_, cloneHasOrigWrite := clone.Headers[3]
	req.False(cloneHasOrigWrite, "writing the original's headers must not mutate the clone")
}
