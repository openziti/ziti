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

package routes

import (
	"testing"

	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

// Test_routerRef_RendersAResolvedRouter: the ordinary case, where the end is a router this controller holds.
func Test_routerRef_RendersAResolvedRouter(t *testing.T) {
	req := require.New(t)

	ref := routerRef("r1", model.NewRouter("r1", "router-one", "", 0, false))

	req.NotNil(ref)
	req.Equal("r1", ref.ID)
	req.Equal("router-one", ref.Name)
	req.Contains(ref.Links, EntityNameSelf)
}

// Test_routerRef_RendersAnUnresolvedDestinationById is why this exists: a link can name a destination this
// controller has not caught up to, or one deleted since the link was recorded. The link still has to render,
// since failing fails the whole listing rather than the one row, so the id stands in for what is not known.
func Test_routerRef_RendersAnUnresolvedDestinationById(t *testing.T) {
	req := require.New(t)

	ref := routerRef("not-applied-yet", nil)

	req.NotNil(ref, "a row naming a destination that cannot be resolved must still render")
	req.Equal("not-applied-yet", ref.ID)
	req.Empty(ref.Name, "there is nothing to read a name from")
	req.Contains(ref.Links, EntityNameSelf, "the id is enough to build the self link from")
}

// Test_routerRef_OmitsAnEndItCannotName: a link records its destination's id but not its source's, so a
// source holding no router cannot even be named. Absent is the truthful rendering; an empty id would not be.
func Test_routerRef_OmitsAnEndItCannotName(t *testing.T) {
	require.Nil(t, routerRef("", nil))
}
