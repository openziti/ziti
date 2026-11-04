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

package network

import (
	"testing"

	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

// TestRemoveLink_ReportsWhetherItKnewTheLink: an id the controller does not hold is still acted on, since its
// fault goes to every connected router, so the answer is the only thing that tells a removed link from one
// that never existed.
func TestRemoveLink_ReportsWhetherItKnewTheLink(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	link := model.NewTestLink("known", newPersistedRouter(t, network, addr, "src"), newPersistedRouter(t, network, addr, "dst"))
	network.Link.Add(link)

	req.True(network.RemoveLink(link.Id), "a link the controller holds is known")
	_, found := network.Link.Get(link.Id)
	req.False(found, "and is removed")

	req.False(network.RemoveLink("never-existed"), "an id the controller does not hold is not")
}
