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
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"google.golang.org/protobuf/proto"
)

// updateLinkListenersHandler receives mid-session listener updates from a
// router and propagates the new state. Router sends this whenever its
// link subsystem rebuilds (managed config Apply, local YAML reload,
// etc.). Hello carries the initial snapshot at connect time; this handler
// covers everything after.
type updateLinkListenersHandler struct {
	router  *model.Router
	network *network.Network
}

func newUpdateLinkListenersHandler(router *model.Router, network *network.Network) *updateLinkListenersHandler {
	return &updateLinkListenersHandler{router: router, network: network}
}

func (self *updateLinkListenersHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_UpdateLinkListenersType)
}

func (self *updateLinkListenersHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).WithField("routerId", self.router.Id)

	listeners := &ctrl_pb.Listeners{}
	if err := proto.Unmarshal(msg.Body, listeners); err != nil {
		log.WithError(err).Error("unable to unmarshal UpdateLinkListeners")
		return
	}

	self.router.SetLinkListeners(listeners.Listeners)
	log.WithField("listenerCount", len(listeners.Listeners)).
		Info("updated router link listeners; redistributing to peers")

	// Trigger the existing peer-redistribution path: peers receive a
	// PeerStateChange carrying this router's new listeners and update
	// their local dial decisions.
	self.network.RouterMessaging.RouterListenersUpdated(self.router)
}
