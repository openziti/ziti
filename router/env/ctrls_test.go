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

package env

import (
	"testing"

	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/stretchr/testify/require"
)

// Test_firstUnderlayHeaders verifies that the initial underlay is flagged as the first grouped
// connection, and that each call yields an independent map. If the flag leaked onto headers shared with
// additional underlays (e.g. ctrl.high), every additional underlay would look like a new channel and
// trip the controller's already-connected churn guard.
func Test_firstUnderlayHeaders(t *testing.T) {
	req := require.New(t)

	first := firstUnderlayHeaders()

	firstFlag, ok := first.GetBoolHeader(channel.IsFirstGroupConnection)
	req.True(ok, "initial underlay headers must contain the first-connection flag")
	req.True(firstFlag)

	grouped, _ := first.GetBoolHeader(channel.IsGroupedHeader)
	req.True(grouped, "initial underlay must be flagged as grouped")
	chType, _ := first.GetStringHeader(channel.TypeHeader)
	req.Equal(ctrlchan.ChannelTypeDefault, chType, "initial underlay must carry the default ctrl channel type")

	// Callers mutate the returned headers, so each call must hand back a distinct map; a shared map
	// would let the first-connection flag reach underlays that must not carry it.
	other := firstUnderlayHeaders()
	delete(other, channel.IsFirstGroupConnection)
	_, stillSet := first[channel.IsFirstGroupConnection]
	req.True(stillSet, "each call must return an independent header map")
}
