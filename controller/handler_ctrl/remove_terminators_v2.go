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
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"google.golang.org/protobuf/proto"
)

// removeTerminatorsV2Handler handles RemoveTerminatorsV2Request messages. It performs the same
// batch removal as removeTerminatorsHandler, but answers asynchronously with a
// RemoveTerminatorsV2Response instead of a synchronous result, so the router does not hold a
// request slot open waiting for the reply.
type removeTerminatorsV2Handler struct {
	baseHandler
}

func newRemoveTerminatorsV2Handler(network *network.Network, router *model.Router) *removeTerminatorsV2Handler {
	return &removeTerminatorsV2Handler{
		baseHandler: baseHandler{
			router:  router,
			network: network,
		},
	}
}

func (self *removeTerminatorsV2Handler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_RemoveTerminatorsV2RequestType)
}

func (self *removeTerminatorsV2Handler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	request := &ctrl_pb.RemoveTerminatorsV2Request{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		log.WithError(err).Error("failed to unmarshal remove terminators v2 message")
		return
	}

	go self.handleRemoveTerminators(ch, request)
}

func (self *removeTerminatorsV2Handler) handleRemoveTerminators(ch channel.Channel, request *ctrl_pb.RemoveTerminatorsV2Request) {
	log := pfxlog.ContextLogger(ch.Label()).WithField("requestId", request.RequestId)

	response := &ctrl_pb.RemoveTerminatorsV2Response{RequestId: request.RequestId}

	toDelete := self.filterConfirmedAbsent(request)

	// Everything was a confirmed no-op (already gone), so there's nothing to order through raft.
	if len(toDelete) == 0 {
		response.Success = true
		self.sendResponse(ch, response)
		return
	}

	if err := self.network.Terminator.DeleteBatch(toDelete, self.newChangeContext(ch, "fabric.remove.terminators.batch.v2")); err == nil {
		log.
			WithField("routerId", ch.Id()).
			WithField("terminatorIds", toDelete).
			Info("removed terminators")
		response.Success = true
	} else if command.WasRateLimited(err) {
		log.WithError(err).WithField("terminatorIds", toDelete).
			Info("unable to remove terminators, rate limited")
		response.WasRateLimited = true
		response.Msg = err.Error()
	} else {
		log.WithError(err).WithField("terminatorIds", toDelete).
			Error("unable to remove terminators")
		response.Msg = err.Error()
	}

	self.sendResponse(ch, response)
}

// filterConfirmedAbsent returns the terminator ids that must go through the ordered (raft) delete.
// On the leader, where the applied state is current, it drops ids the router confirmed were created
// (createConfirmed) but that no longer exist: those deletes are confirmed no-ops, so skipping them
// keeps a storm of no-op retries from consuming the raft/command rate limiter. Ids that still exist,
// or whose create the router did not confirm, are kept: an unconfirmed create may be committed but
// not yet applied, so an absent id doesn't prove it's gone, and sending it through raft orders the
// delete after any such create (ApplyDeleteBatch handles non-existent ids gracefully). Off the leader
// every id is kept, since a lagging follower can't prove an id is gone.
func (self *removeTerminatorsV2Handler) filterConfirmedAbsent(request *ctrl_pb.RemoveTerminatorsV2Request) []string {
	kept, skipped := filterConfirmedAbsentTerminators(
		request.TerminatorIds,
		request.CreateConfirmed,
		self.network.Dispatcher.IsLeader(),
		self.network.Terminator.IsEntityPresent,
	)

	if skipped > 0 {
		pfxlog.Logger().
			WithField("requestId", request.RequestId).
			WithField("skipped", skipped).
			WithField("remaining", len(kept)).
			Debug("leader fast-path skipped confirmed already-removed terminators")
	}

	return kept
}

// filterConfirmedAbsentTerminators implements the leader fast-path filter for filterConfirmedAbsent,
// factored out so it can be unit tested without a live network. See filterConfirmedAbsent for the
// rationale. createConfirmed is index-aligned with ids; a missing entry counts as false. isPresent is
// only consulted for confirmed ids, and an error from it keeps the id (safe default).
func filterConfirmedAbsentTerminators(ids []string, createConfirmed []bool, isLeader bool, isPresent func(string) (bool, error)) (kept []string, skipped int) {
	if !isLeader {
		return ids, 0
	}

	kept = make([]string, 0, len(ids))
	for i, id := range ids {
		confirmed := i < len(createConfirmed) && createConfirmed[i]
		if confirmed {
			if present, err := isPresent(id); err == nil && !present {
				skipped++
				continue
			}
		}
		kept = append(kept, id)
	}

	return kept, skipped
}

func (self *removeTerminatorsV2Handler) sendResponse(ch channel.Channel, response *ctrl_pb.RemoveTerminatorsV2Response) {
	if err := protobufs.MarshalTyped(response).Send(ch); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).WithField("requestId", response.RequestId).
			Error("failed to send remove terminators v2 response")
	}
}
