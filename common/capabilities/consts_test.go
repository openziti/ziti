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

package capabilities

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRouterCapabilityBitNumbering locks the shared router capability bit
// positions. MultiChannel must stay bit 1 for control-channel back-compat with
// released routers/controllers; ConnectV2 is bit 2 in the unified namespace.
func TestRouterCapabilityBitNumbering(t *testing.T) {
	req := require.New(t)
	req.Equal(RouterCapability(1), RouterMultiChannel)
	req.Equal(RouterCapability(2), RouterConnectV2)
	req.Equal(RouterCapability(3), RouterDataModel)
	req.Equal(RouterCapability(4), RouterServiceSubscriptions)
}

func TestGetRouterCapabilitiesMask(t *testing.T) {
	req := require.New(t)
	mask := GetRouterCapabilitiesMask()
	req.True(mask.IsSet(RouterMultiChannel))
	req.True(mask.IsSet(RouterConnectV2))
	req.True(mask.IsSet(RouterPostureChecks))
	req.True(mask.IsSet(RouterBindSuccess))
	req.True(mask.IsSet(RouterServiceSubscriptions))
	req.True(mask.IsSet(RouterDataModel))
	req.False(mask.IsSet(0))
}
