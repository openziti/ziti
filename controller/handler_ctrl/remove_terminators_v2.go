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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
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

	toDelete := self.selectRemovableTerminators(request.TerminatorIds, request.CreateConfirmed)

	// Everything was either not owned by this router or a confirmed no-op (already gone), so there's
	// nothing to order through raft.
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
	} else if wasControllerBusy(err) {
		log.WithError(err).WithField("terminatorIds", toDelete).
			Info("unable to remove terminators, controller busy")
		response.WasRateLimited = true
		response.Msg = err.Error()
	} else {
		log.WithError(err).WithField("terminatorIds", toDelete).
			Error("unable to remove terminators")
		response.Msg = err.Error()
	}

	self.sendResponse(ch, response)
}

// sendResponse queues the response, giving up if the channel's send queue stays full. A response is
// only ever logged here, and the router recovers a lost one through rate-limiter expiry and its retry
// scan, so blocking a per-request goroutine on an undrained queue buys nothing.
func (self *removeTerminatorsV2Handler) sendResponse(ch channel.Channel, response *ctrl_pb.RemoveTerminatorsV2Response) {
	if err := protobufs.MarshalTyped(response).WithTimeout(5 * time.Second).Send(ch); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).WithField("requestId", response.RequestId).
			Error("failed to send remove terminators v2 response")
	}
}
