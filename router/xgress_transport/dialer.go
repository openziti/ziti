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

package xgress_transport

import (
	"fmt"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
)

type dialer struct {
	id      *identity.TokenId
	ctrl    xgress.CtrlChannel
	options *xgress.Options
	tcfg    transport.Configuration
}

func (txd *dialer) IsTerminatorValid(string, string) bool {
	return true
}

func newDialer(id *identity.TokenId, ctrl xgress.CtrlChannel, options *xgress.Options, tcfg transport.Configuration) (xgress.Dialer, error) {
	txd := &dialer{
		id:      id,
		ctrl:    ctrl,
		options: options,
		tcfg:    tcfg,
	}
	return txd, nil
}

func (txd *dialer) Dial(destination string, sessionId *identity.TokenId, address xgress.Address, bindHandler xgress.BindHandler) (xt.PeerData, error) {
	txDestination, err := transport.ParseAddress(destination)
	if err != nil {
		return nil, fmt.Errorf("cannot dial on invalid address [%s] (%s)", destination, err)
	}

	peer, err := txDestination.Dial("x/"+sessionId.Token, sessionId, txd.tcfg)
	if err != nil {
		return nil, err
	}

	conn := &transportXgresscConn{peer}
	x := xgress.NewXgress(sessionId, address, conn, xgress.Terminator, txd.options)
	bindHandler.HandleXgressBind(x)
	x.Start()

	return nil, nil
}
