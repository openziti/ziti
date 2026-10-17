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

package raft

import (
	"sync"

	"github.com/hashicorp/raft"
	"github.com/openziti/ziti/v2/common/capabilities"
)

// clusterCapabilityTracker answers which capabilities every member of the raft cluster supports.
//
// A mask rather than one answer per feature, because features arrive in different releases: a cluster
// part-way through an upgrade can support link gossip while not yet supporting whatever the next release
// adds, and collapsing that to a single "can this cluster gossip" answer would switch a shipped feature off
// during the upgrade after the one that shipped it.
//
// It remembers what each member last advertised rather than reading the currently-connected peers. The
// mesh's peer map holds only connected peers, so asking it directly would call a cluster uniform while a
// peer it cannot reach runs an older version, and would change the answer on every partition. A controller
// cannot change what it advertises without disconnecting and reconnecting, so the last mask seen for a
// member holds until it comes back with a new one.
//
// Concurrent use is safe.
type clusterCapabilityTracker struct {
	lock  sync.RWMutex
	masks map[raft.ServerID]*capabilities.Mask[capabilities.ControllerCapability]
}

// newClusterCapabilityTracker seeds the tracker with this controller's own advertised capabilities.
// Membership includes us, so we have to be in the map for any capability to come back supported.
func newClusterCapabilityTracker(selfId raft.ServerID) *clusterCapabilityTracker {
	return &clusterCapabilityTracker{
		masks: map[raft.ServerID]*capabilities.Mask[capabilities.ControllerCapability]{
			selfId: capabilities.GetControllerCapabilitiesMask(),
		},
	}
}

// observe records what a member advertised. Driven by the mesh as peers become known, not by a read: a peer
// that connected and disconnected between two reads would otherwise never be seen at all, so an older
// controller could join and drop unnoticed and leave the cluster looking more capable than it is.
func (self *clusterCapabilityTracker) observe(id raft.ServerID, mask *capabilities.Mask[capabilities.ControllerCapability]) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.masks[id] = mask
}

// clusterMask returns the capabilities every given member supports, as the intersection of what they
// advertised, and whether that answer is known at all.
//
// Not known until every member has been observed. A member nothing has been heard from could be running
// anything, and the difference between that and "a member that supports nothing" matters: a controller that
// has just restarted has observed only itself, and reporting an empty mask there would tell every router
// connected to it that the cluster cannot carry a feature its peers are still carrying happily. Withholding
// instead leaves routers on the last answer they had, which is the right one across a restart.
//
// An empty membership is not known either, since it means the raft configuration could not be read.
func (self *clusterCapabilityTracker) clusterMask(members []raft.ServerID) (*capabilities.Mask[capabilities.ControllerCapability], bool) {
	if len(members) == 0 {
		return nil, false
	}

	self.lock.RLock()
	defer self.lock.RUnlock()

	var result *capabilities.Mask[capabilities.ControllerCapability]
	for _, id := range members {
		mask, seen := self.masks[id]
		if !seen {
			return nil, false
		}
		if result == nil {
			result = mask
			continue
		}
		result = result.And(mask)
	}

	if result == nil {
		return capabilities.NewMask[capabilities.ControllerCapability](), true
	}
	return result, true
}
