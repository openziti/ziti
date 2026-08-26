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

// selectOwnedTerminators returns the terminator ids from a remove request that this router may
// remove, dropping (and logging) any whose terminator is owned by a different router, so a router
// can only remove terminators it owns. Absent ids are kept so a delete racing a not-yet-applied
// create is still ordered after it.
func (self *baseHandler) selectOwnedTerminators(ids []string) []string {
	kept, rejected := filterOwnedTerminators(ids, self.router.Id, self.lookupTerminatorOwner)
	if rejected > 0 {
		pfxlog.Logger().
			WithField("routerId", self.router.Id).
			WithField("rejected", rejected).
			Warn("router attempted to remove terminators it does not own; rejected")
	}
	return kept
}

// filterOwnedTerminators returns the ids owned by requestingRouterId (or not currently present),
// dropping ids whose terminator resolves to a different router; rejected counts those drops. A
// lookup error keeps the id, so an unresolved owner is not treated as an ownership violation.
func filterOwnedTerminators(ids []string, requestingRouterId string, lookup func(string) (routerId string, present bool, err error)) (kept []string, rejected int) {
	kept = make([]string, 0, len(ids))
	for _, id := range ids {
		routerId, present, err := lookup(id)
		if err == nil && present && routerId != requestingRouterId {
			rejected++
			continue
		}
		kept = append(kept, id)
	}
	return kept, rejected
}
