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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/channel"
)

type closeHandler struct {
	r       *network.Router
	network *network.Network
}

func newCloseHandler(r *network.Router, network *network.Network) *closeHandler {
	return &closeHandler{r: r, network: network}
}

func (h *closeHandler) HandleClose(channel.Channel) {
	pfxlog.Logger().WithField("routerId", h.r.Id).Warn("disconnected")
	h.network.DisconnectRouter(h.r)
}

type xctrlCloseHandler struct {
	done chan struct{}
}

func newXctrlCloseHandler(done chan struct{}) channel.CloseHandler {
	return &xctrlCloseHandler{done: done}
}

func (h *xctrlCloseHandler) HandleClose(ch channel.Channel) {
	pfxlog.ContextLogger(ch.Label()).Info("closing Xctrl instances")
	close(h.done)
}
