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

	// Don't pre-filter by IsEntityPresent here. The create for a terminator may be
	// in-flight in raft but not yet applied to the DB. If we skip it here, the create
	// will apply after we return success, leaving an orphan. By sending all IDs through
	// raft, the delete will be ordered after the create and ApplyDeleteBatch will handle
	// non-existent IDs gracefully.
	if len(request.TerminatorIds) == 0 {
		self.sendResponse(ch, &ctrl_pb.RemoveTerminatorsV2Response{RequestId: request.RequestId, Success: true})
		return
	}

	response := &ctrl_pb.RemoveTerminatorsV2Response{RequestId: request.RequestId}

	if err := self.network.Terminator.DeleteBatch(request.TerminatorIds, self.newChangeContext(ch, "fabric.remove.terminators.batch.v2")); err == nil {
		log.
			WithField("routerId", ch.Id()).
			WithField("terminatorIds", request.TerminatorIds).
			Info("removed terminators")
		response.Success = true
	} else if command.WasRateLimited(err) {
		log.WithError(err).WithField("terminatorIds", request.TerminatorIds).
			Info("unable to remove terminators, rate limited")
		response.WasRateLimited = true
		response.Msg = err.Error()
	} else {
		log.WithError(err).WithField("terminatorIds", request.TerminatorIds).
			Error("unable to remove terminators")
		response.Msg = err.Error()
	}

	self.sendResponse(ch, response)
}

func (self *removeTerminatorsV2Handler) sendResponse(ch channel.Channel, response *ctrl_pb.RemoveTerminatorsV2Response) {
	if err := protobufs.MarshalTyped(response).Send(ch); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).WithField("requestId", response.RequestId).
			Error("failed to send remove terminators v2 response")
	}
}
