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
	"errors"
	"testing"

	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

// newUnresolvedLink returns a link from router-one whose destination it does not hold, as for a destination
// that is not connected.
func newUnresolvedLink(destId string) *model.Link {
	return &model.Link{Id: "l1", Src: model.NewRouter("r1", "router-one", "", 0, false), DstId: destId}
}

// Test_linkToRestModel_RendersALinkWhoseDestinationIsGone: a link can outlive its destination router until
// the controller retires it, and the link listing renders every link through this. Failing the row fails the
// whole listing, so a destination that is no longer there renders by its id.
func Test_linkToRestModel_RendersALinkWhoseDestinationIsGone(t *testing.T) {
	req := require.New(t)

	detail, err := linkToRestModel(newUnresolvedLink("deleted-router"), func(id string) (*model.Router, error) {
		return nil, boltz.NewNotFoundError("router", "id", id)
	})

	req.NoError(err, "a link to a deleted router must still render")
	req.Equal("l1", *detail.ID)
	req.Equal("router-one", detail.SourceRouter.Name)
	req.Equal("deleted-router", detail.DestRouter.ID)
	req.Empty(detail.DestRouter.Name, "there is nothing to read a name from")
	req.Contains(detail.DestRouter.Links, EntityNameSelf, "the id is enough to build the self link from")
}

// Test_linkToRestModel_RendersAReadDestination: a destination the link does not hold is read, and renders as
// the router it is.
func Test_linkToRestModel_RendersAReadDestination(t *testing.T) {
	req := require.New(t)

	detail, err := linkToRestModel(newUnresolvedLink("r2"), func(id string) (*model.Router, error) {
		return model.NewRouter(id, "router-two", "", 0, false), nil
	})

	req.NoError(err)
	req.Equal("r2", detail.DestRouter.ID)
	req.Equal("router-two", detail.DestRouter.Name)
}

// Test_linkToRestModel_ReturnsAFailedRead: only a destination that is not found is a miss. Any other read
// error says nothing about the destination, and is returned rather than rendered as a missing router.
func Test_linkToRestModel_ReturnsAFailedRead(t *testing.T) {
	readErr := errors.New("store unavailable")

	_, err := linkToRestModel(newUnresolvedLink("r2"), func(string) (*model.Router, error) {
		return nil, readErr
	})

	require.ErrorIs(t, err, readErr)
}
