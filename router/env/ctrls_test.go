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

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/stretchr/testify/require"
)

// Test_firstUnderlayHeaders verifies that only the initial underlay is flagged as the first grouped
// connection. If the flag leaked onto the base headers, every additional underlay (e.g. ctrl.high)
// would look like a new channel and trip the controller's already-connected churn guard.
func Test_firstUnderlayHeaders(t *testing.T) {
	req := require.New(t)

	base := channel.Headers{}
	base.PutBoolHeader(channel.IsGroupedHeader, true)
	base.PutStringHeader(channel.TypeHeader, ctrlchan.ChannelTypeDefault)

	first := firstUnderlayHeaders(base)

	// The base headers, reused for additional underlays, must not carry the first-connection flag.
	_, baseHasFirst := base[channel.IsFirstGroupConnection]
	req.False(baseHasFirst, "base headers for additional underlays must not be flagged as first")

	// The initial underlay's headers must carry it.
	firstFlag, ok := first.GetBoolHeader(channel.IsFirstGroupConnection)
	req.True(ok, "initial underlay headers must contain the first-connection flag")
	req.True(firstFlag)

	// The remaining base headers are preserved on the copy.
	grouped, _ := first.GetBoolHeader(channel.IsGroupedHeader)
	req.True(grouped, "grouped flag preserved on the initial underlay copy")
	chType, _ := first.GetStringHeader(channel.TypeHeader)
	req.Equal(ctrlchan.ChannelTypeDefault, chType, "channel type preserved on the initial underlay copy")
}
