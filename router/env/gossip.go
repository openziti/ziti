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
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/router/xlink"
)

// LinkGossipNotifier sends link state changes as gossip deltas to the
// subscription controller. Used by the link registry when all controllers
// support link gossip.
type LinkGossipNotifier interface {
	// NextVersion returns the next Lamport clock version. Call this from the
	// event loop when collecting links for notification, so the version
	// reflects event ordering rather than send ordering.
	NextVersion() uint64
	// NotifyLinks sends gossip entries for new or updated links. Each link
	// carries a pre-assigned version from NextVersion. Returns an error if
	// the subscription controller was unavailable or the send failed.
	NotifyLinks(links []VersionedLink) error
	// NotifyLinkFault sends a gossip tombstone for a faulted link. Returns an
	// error if the subscription controller was unavailable or the send failed.
	NotifyLinkFault(linkId string, iteration uint32) error
	// HandleDigest processes a gossip digest from the controller and responds
	// with entries the controller is missing plus tombstones for dead links.
	HandleDigest(msg *channel.Message, ch channel.Channel)
}

// VersionedLink pairs a link with a pre-assigned gossip version.
type VersionedLink struct {
	Link    xlink.Xlink
	Version uint64
}
