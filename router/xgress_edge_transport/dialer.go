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

package xgress_edge_transport

import (
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/pkg/errors"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/router/xgress_common"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/transport/v2"
)

type dialer struct {
	options *xgress.Options
}

func (txd *dialer) IsTerminatorValid(string, string) bool {
	return true
}

func newDialer(options *xgress.Options) (xgress.Dialer, error) {
	txd := &dialer{
		options: options,
	}
	return txd, nil
}

func (txd *dialer) Dial(params xgress.DialParams) (xt.PeerData, error) {
	destination := params.GetDestination()
	circuitId := params.GetCircuitId()
	log := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(params.GetLogContext()).
		WithField("binding", "edge_transport").
		WithField("destination", destination)

	txDestination, err := transport.ParseAddress(destination)
	if err != nil {
		return nil, xgress.MisconfiguredTerminatorError{InnerError: errors.Wrapf(err, "cannot dial on invalid address [%s]", destination)}
	}

	log.Debug("dialing")
	to := txd.options.ConnectTimeout
	timeToDeadline := time.Until(params.GetDeadline())
	if timeToDeadline > 0 && timeToDeadline < to {
		to = timeToDeadline
	}
	peer, err := txDestination.Dial("x/"+circuitId.Token, circuitId, to, nil)
	if err != nil {
		return nil, err
	}

	log.Debugf("successful connection to %v from %v (s/%v)", destination, peer.LocalAddr(), circuitId.Token)

	xgConn := xgress_common.NewXgressConn(peer, true, true)
	peerData := make(xt.PeerData, 3)
	if peerKey, ok := circuitId.Data[edge.PublicKeyHeader]; ok {
		if publicKey, err := xgConn.SetupServerCrypto(peerKey); err != nil {
			return nil, err
		} else {
			peerData[edge.PublicKeyHeader] = publicKey
		}
	}

	peerData[uint32(ctrl_msg.TerminatorLocalAddressHeader)] = []byte(peer.LocalAddr().String())
	peerData[uint32(ctrl_msg.TerminatorRemoteAddressHeader)] = []byte(peer.RemoteAddr().String())

	x := xgress.NewXgress(circuitId.Token, params.GetCtrlId(), params.GetAddress(), xgConn, xgress.Terminator, txd.options, params.GetCircuitTags())
	params.GetBindHandler().HandleXgressBind(x)
	x.Start()

	return peerData, nil
}
