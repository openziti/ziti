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

package state

import (
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/xlink"
)

// LinkDestSubscriber adapts an xlink.Registry to common.RouterEventSubscriber: a router leaving the
// router data model means the link destination naming it is gone, and the links to it with it.
//
// The controller's PeerState_Removed notification covers the ordinary case. This covers the one it
// cannot: a router deleted while this router was disconnected from the controller is only ever visible
// as its absence from the next full state. A link to a deleted router can never be repaired, since a
// recreated router is assigned a new id.
type LinkDestSubscriber struct {
	xlink.Registry
}

// OnRouterChanged implements common.RouterEventSubscriber. Link destinations come from the controller's
// peer-state notifications, not from RDM router records, so there is nothing to do when one is created
// or updated.
func (self LinkDestSubscriber) OnRouterChanged(*edge_ctrl_pb.DataState_Router) {}

// OnRouterRemoved implements common.RouterEventSubscriber. Removing a destination the registry does not
// know about is a no-op, so unknown ids need no filtering.
func (self LinkDestSubscriber) OnRouterRemoved(routerId string) {
	self.RemoveLinkDest(routerId)
}

// ensure interface compliance
var _ common.RouterEventSubscriber = LinkDestSubscriber{}
