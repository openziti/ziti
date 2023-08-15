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
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"github.com/openziti/fabric/router/env"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type peerStateChangeHandler struct {
	env env.RouterEnv
}

func newPeerStateChangeHandler(env env.RouterEnv) *peerStateChangeHandler {
	handler := &peerStateChangeHandler{
		env: env,
	}

	return handler
}

func (self *peerStateChangeHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_PeerStateChangeRequestType)
}

func (self *peerStateChangeHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	peerStateChanges := &ctrl_pb.PeerStateChanges{}
	if err := proto.Unmarshal(msg.Body, peerStateChanges); err != nil {
		logrus.WithError(err).Error("error unmarshalling peer state change message")
		return
	}

	go self.DispatchChanges(peerStateChanges)
}

func (self *peerStateChangeHandler) DispatchChanges(changes *ctrl_pb.PeerStateChanges) {
	log := pfxlog.Logger()
	for _, peerStateChange := range changes.Changes {
		if peerStateChange.Id == self.env.GetRouterId().Token {
			log.WithField("routerId", peerStateChange.Id).Warn("got peer state change message for myself, ignoring")
		} else if peerStateChange.State == ctrl_pb.PeerState_Removed {
			self.env.GetXlinkRegistry().RemoveLinkDest(peerStateChange.Id)
		} else {
			self.env.GetXlinkRegistry().UpdateLinkDest(peerStateChange.Id, peerStateChange.Version, peerStateChange.State == ctrl_pb.PeerState_Healthy, peerStateChange.Listeners)
		}
	}
}
