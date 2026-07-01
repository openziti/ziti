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
	req.Equal(1, RouterMultiChannel)
	req.Equal(2, RouterConnectV2)
}

func TestGetRouterCapabilitiesMask(t *testing.T) {
	req := require.New(t)
	mask := GetRouterCapabilitiesMask()
	req.True(IsSet(mask, RouterMultiChannel))
	req.True(IsSet(mask, RouterConnectV2))
	req.False(IsSet(mask, 0))
	req.False(IsSet(mask, 3))
}
