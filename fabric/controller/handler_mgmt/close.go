/*
	Copyright 2019 Netfoundry, Inc.

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

package handler_mgmt

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/michaelquigley/pfxlog"
)

type xmgmtCloseHandler struct {
	done chan struct{}
}

func newXmgmtCloseHandler(done chan struct{}) channel2.CloseHandler {
	return &xmgmtCloseHandler{done: done}
}

func (h *xmgmtCloseHandler) HandleClose(ch channel2.Channel) {
	pfxlog.Logger().Infof("closing Xmgmt instances for [%s]", ch.Label())
	close(h.done)
}
