/*
	Copyright NetFoundry, Inc.

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

package handler_edge_ctrl

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/foundation/channel2"
)

type helloHandler struct {
	appEnv *env.AppEnv
}

func NewHelloHandler(appEnv *env.AppEnv) *helloHandler {
	return &helloHandler{
		appEnv: appEnv,
	}
}

func (h *helloHandler) ContentType() int32 {
	return env.ClientHelloType
}

func (h *helloHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	respHello := &edge_ctrl_pb.ClientHello{}
	if err := proto.Unmarshal(msg.Body, respHello); err != nil {
		pfxlog.Logger().WithError(err).Error("could not unmarshal clientHello after serverHello")
		return
	}

	r, err := h.appEnv.GetHostController().GetNetwork().GetRouter(ch.Id().Token)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not find router %v, closing channel", ch.Id().Token)
		_ = ch.Close()
		return
	}

	if r.Fingerprint == nil {
		pfxlog.Logger().Errorf("router %v is not enrolled, closing channel", ch.Id().Token)
		_ = ch.Close()
		return
	}

	edgeRouter, _ := h.appEnv.Handlers.EdgeRouter.ReadOneByFingerprint(*r.Fingerprint)
	if edgeRouter == nil {
		pfxlog.Logger().Errorf("edge hello received for non-edge router %v", ch.Id().Token)
		return
	}

	h.appEnv.Broker.ReceiveHello(r, edgeRouter, respHello)
}
