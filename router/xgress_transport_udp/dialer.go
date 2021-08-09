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

package xgress_transport_udp

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xgress_udp"
	"github.com/openziti/foundation/identity/identity"
	"net"
)

func (txd *dialer) Dial(destination string, circuitId *identity.TokenId, address xgress.Address, bindHandler xgress.BindHandler, ctx logcontext.Context) (xt.PeerData, error) {
	log := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(ctx).
		WithField("binding", "transport_udp").
		WithField("destination", destination)

	log.Debugf("parsing %v for xgress address: %v", destination, address)
	packetAddress, err := xgress_udp.Parse(destination)
	if err != nil {
		return nil, fmt.Errorf("cannot dial on invalid address [%s] (%w)", destination, err)
	}

	log.Debugf("dialing packet address [%v]", packetAddress)
	conn, err := net.Dial(packetAddress.Network(), packetAddress.Address())
	if err != nil {
		return nil, err
	}

	log.Infof("bound on [%v]", conn.LocalAddr())

	x := xgress.NewXgress(circuitId, address, newPacketConn(conn), xgress.Terminator, txd.options)
	bindHandler.HandleXgressBind(x)
	x.Start()

	return nil, nil
}

func (txd *dialer) IsTerminatorValid(string, string) bool {
	return true
}

func newDialer(id *identity.TokenId, ctrl xgress.CtrlChannel, options *xgress.Options) (*dialer, error) {
	txd := &dialer{
		id:      id,
		ctrl:    ctrl,
		options: options,
	}
	return txd, nil
}

type dialer struct {
	id      *identity.TokenId
	ctrl    xgress.CtrlChannel
	options *xgress.Options
}
