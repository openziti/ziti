/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/foundation/identity/identity"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLifecycle(t *testing.T) {
	linkController := newLinkController()

	r0 := NewRouter("r0", "", "")
	r1 := NewRouter("r1", "", "")
	l0 := &Link{
		Id:  &identity.TokenId{Token: "l0"},
		Src: r0,
		Dst: r1,
	}

	linkController.add(l0)
	assert.True(t, linkController.has(l0))

	links := r0.routerLinks.GetLinks()
	assert.Equal(t, 1, len(links))
	assert.Equal(t, l0, links[0])

	links = r1.routerLinks.GetLinks()
	assert.Equal(t, 1, len(links))
	assert.Equal(t, l0, links[0])

	linkController.remove(l0)
	assert.False(t, linkController.has(l0))

	links = r0.routerLinks.GetLinks()
	assert.Equal(t, 0, len(links))

	links = r1.routerLinks.GetLinks()
	assert.Equal(t, 0, len(links))
}

func TestNeighbors(t *testing.T) {
	linkController := newLinkController()

	r0 := newRouterForTest("r0", "", nil, nil)
	r1 := newRouterForTest("r1", "", nil, nil)
	l0 := &Link{
		Id:  &identity.TokenId{Token: "l0"},
		Src: r0,
		Dst: r1,
	}
	l0.addState(newLinkState(Connected))
	linkController.add(l0)

	neighbors := linkController.connectedNeighborsOfRouter(r0)
	assert.Equal(t, 1, len(neighbors))
	assert.Equal(t, r1, neighbors[0])
}
