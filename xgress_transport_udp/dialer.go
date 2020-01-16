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

package xgress_transport_udp

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-fabric/xgress_udp"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"net"
)

type dialer struct {
	id      *identity.TokenId
	ctrl    xgress.CtrlChannel
	options *xgress.Options
}

func newDialer(id *identity.TokenId, ctrl xgress.CtrlChannel, options *xgress.Options) (*dialer, error) {
	txd := &dialer{
		id:      id,
		ctrl:    ctrl,
		options: options,
	}
	return txd, nil
}

func (txd *dialer) Dial(destination string, sessionId *identity.TokenId, address xgress.Address, bindHandler xgress.BindHandler) error {
	pfxlog.Logger().Infof("Parsing %v for xgress address: %v", destination, address)
	packetAddress, err := xgress_udp.Parse(destination)
	if err != nil {
		return fmt.Errorf("cannot dial on invalid address [%s] (%s)", destination, err)
	}

	pfxlog.Logger().Infof("Dialing packet address %v", packetAddress)
	conn, err := net.Dial(packetAddress.Network(), packetAddress.Address())
	if err != nil {
		return err
	}

	pfxlog.Logger().Infof("Bound on '%v'", conn.LocalAddr())

	xgConn := &xgressPacketConn{conn}
	x := xgress.NewXgress(sessionId, address, xgConn, xgress.Terminator, txd.options)
	bindHandler.HandleXgressBind(sessionId, address, xgress.Terminator, x)

	return nil
}
