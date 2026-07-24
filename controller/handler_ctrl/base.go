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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
)

type baseHandler struct {
	router  *model.Router
	network *network.Network
}

func (self *baseHandler) newChangeContext(ch channel.Channel, method string) *change.Context {
	return change.NewControlChannelChange(self.router.Id, self.router.Name, method, ch)
}

// ownsTerminator reports whether terminator belongs to the router on the other end of this handler's
// control channel. Terminator operations arriving on the fabric control channel are scoped to the
// requesting router, mirroring the edge control channel's verifyTerminator.
func (self *baseHandler) ownsTerminator(terminator *model.Terminator) bool {
	return terminator.Router == self.router.Id
}

// lookupTerminatorOwner returns the id of the router that owns terminator id and whether the
// terminator currently exists. A not-found terminator returns ("", false, nil); other read errors
// are returned so the caller can default to keeping the id rather than acting on incomplete state.
func (self *baseHandler) lookupTerminatorOwner(id string) (routerId string, present bool, err error) {
	terminator, err := self.network.Terminator.Read(id)
	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return terminator.Router, true, nil
}

// selectRemovableTerminators returns the terminator ids from a remove request that should be sent
// through the ordered delete. It drops ids whose terminator is owned by a different router, so a
// router can only remove terminators it owns, and (on the leader) confirmed-created ids that are
// already gone, so a storm of no-op retries doesn't consume the raft rate limiter. createConfirmed
// may be nil when the request carries no confirmation, in which case no id is treated as confirmed.
func (self *baseHandler) selectRemovableTerminators(ids []string, createConfirmed []bool) []string {
	kept, rejected, skipped := filterRemovableTerminators(
		ids,
		createConfirmed,
		self.router.Id,
		self.network.Dispatcher.IsLeader(),
		self.lookupTerminatorOwner,
	)

	if rejected > 0 {
		pfxlog.Logger().
			WithField("routerId", self.router.Id).
			WithField("rejected", rejected).
			Warn("router attempted to remove terminators it does not own; rejected")
	}

	if skipped > 0 {
		pfxlog.Logger().
			WithField("routerId", self.router.Id).
			WithField("skipped", skipped).
			WithField("remaining", len(kept)).
			Debug("leader fast-path skipped confirmed already-removed terminators")
	}

	return kept
}

// filterRemovableTerminators selects which requested terminator ids to send through the ordered
// (raft) delete for a remove request from requestingRouterId, using lookup to resolve each id's
// owning router and whether it exists. It drops:
//   - rejected: ids whose terminator exists but is owned by a different router; a router may only
//     remove terminators it owns.
//   - skipped: on the leader, ids the router confirmed it created (createConfirmed, index-aligned
//     with ids) that no longer exist; those deletes are confirmed no-ops.
//
// Absent, unconfirmed ids are kept, so a delete racing a not-yet-applied create is still ordered
// after it. A lookup error keeps the id (safe default). Off the leader nothing is skipped as a
// no-op, since a lagging follower can't prove an id is gone, but ownership is still enforced for ids
// that resolve to a different router.
func filterRemovableTerminators(ids []string, createConfirmed []bool, requestingRouterId string, isLeader bool, lookup func(string) (routerId string, present bool, err error)) (kept []string, rejected int, skipped int) {
	kept = make([]string, 0, len(ids))
	for i, id := range ids {
		routerId, present, err := lookup(id)

		if err == nil && present && routerId != requestingRouterId {
			rejected++
			continue
		}

		if isLeader && err == nil && !present {
			if i < len(createConfirmed) && createConfirmed[i] {
				skipped++
				continue
			}
		}

		kept = append(kept, id)
	}

	return kept, rejected, skipped
}
