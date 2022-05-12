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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/transport/v2"
	"github.com/pkg/errors"
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

func (txd *dialer) Dial(destination string, circuitId *identity.TokenId, address xgress.Address, bindHandler xgress.BindHandler, ctx logcontext.Context, deadline time.Time) (xt.PeerData, error) {
	log := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(ctx).
		WithField("binding", "transport").
		WithField("destination", destination)

	txDestination, err := transport.ParseAddress(destination)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot dial on invalid address [%s]", destination)
	}

	log.Debug("dialing")

	to := txd.options.ConnectTimeout
	timeToDeadline := time.Until(deadline)
	if timeToDeadline > 0 && timeToDeadline < to {
		to = timeToDeadline
	}
	peer, err := txDestination.Dial("x/"+circuitId.Token, circuitId, to, txd.tcfg)
	if err != nil {
		return nil, err
	}

	log.Infof("successful connection to %v from %v", destination, peer.LocalAddr())

	conn := &transportXgressConn{Conn: peer}
	x := xgress.NewXgress(circuitId, address, conn, xgress.Terminator, txd.options)
	bindHandler.HandleXgressBind(x)
	x.Start()

	return nil, nil
}
