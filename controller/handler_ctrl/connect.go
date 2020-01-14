/*
	Copyright 2019 NetFoundry, Inc.

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
	"crypto/sha1"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/xctrl"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type ConnectHandler struct {
	network *network.Network
	xctrls  []xctrl.Xctrl
}

func NewConnectHandler(network *network.Network, xctrls []xctrl.Xctrl) *ConnectHandler {
	return &ConnectHandler{network: network, xctrls: xctrls}
}

func (h *ConnectHandler) HandleConnection(hello *channel2.Hello, certificates []*x509.Certificate) error {
	log := pfxlog.ContextLogger(hello.IdToken)

	id := hello.IdToken

	/*
	 * Control channel connections dump the client-supplied certificate details. We'll
	 * soon be using these certificates for router enrollment.
	 */
	fingerprint := ""
	if certificates != nil {
		log.Debugf("peer has [%d] certificates", len(certificates))
		for i, c := range certificates {
			fingerprint = fmt.Sprintf("%x", sha1.Sum(c.Raw))
			log.Debugf("%d): peer certificate fingerprint [%s]", i, fingerprint)
			log.Debugf("%d): peer common name [%s]", i, c.Subject.CommonName)
		}
	} else {
		log.Warnf("peer has no certificates")
	}
	/* */

	if h.network.ConnectedRouter(id) {
		return errors.New("router already connected")
	}

	if r, err := h.network.KnownRouter(id); err == nil {
		if r.Fingerprint != fingerprint {
			return errors.New("unenrolled router")
		}
	} else {
		return errors.New("unenrolled router")
	}

	return nil
}
