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
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/handler_common"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"google.golang.org/protobuf/proto"
)

type removeTerminatorsHandler struct {
	baseHandler
}

func newRemoveTerminatorsHandler(network *network.Network, router *model.Router) *removeTerminatorsHandler {
	return &removeTerminatorsHandler{
		baseHandler: baseHandler{
			router:  router,
			network: network,
		},
	}
}

func (self *removeTerminatorsHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_RemoveTerminatorsRequestType)
}

func (self *removeTerminatorsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	request := &ctrl_pb.RemoveTerminatorsRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		log.WithError(err).Error("failed to unmarshal remove terminator message")
		return
	}

	go self.handleRemoveTerminators(msg, ch, request)
}

func (self *removeTerminatorsHandler) handleRemoveTerminators(msg *channel.Message, ch channel.Channel, request *ctrl_pb.RemoveTerminatorsRequest) {
	log := pfxlog.ContextLogger(ch.Label())

	// Don't pre-filter by IsEntityPresent here. The create for a terminator may be
	// in-flight in raft but not yet applied to the DB. If we skip it here, the create
	// will apply after we return success, leaving an orphan. By sending all IDs through
	// raft, the delete will be ordered after the create and ApplyDeleteBatch will handle
	// non-existent IDs gracefully.
	if len(request.TerminatorIds) == 0 {
		handler_common.SendSuccess(msg, ch, "")
		return
	}

	if err := self.network.Terminator.DeleteBatch(request.TerminatorIds, self.newChangeContext(ch, "fabric.remove.terminators.batch")); err == nil {
		log.
			WithField("routerId", ch.Id()).
			WithField("terminatorIds", request.TerminatorIds).
			Info("removed terminators")
		handler_common.SendSuccess(msg, ch, "")
	} else if command.WasRateLimited(err) {
		handler_common.SendServerBusy(msg, ch, "remove.terminators")
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}
