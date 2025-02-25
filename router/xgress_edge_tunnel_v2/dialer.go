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

package xgress_edge_tunnel_v2

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/ctrl_msg"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/openziti/ziti/controller/xt"
	"github.com/openziti/ziti/router/xgress"
	"github.com/openziti/ziti/router/xgress_common"
	"github.com/openziti/ziti/tunnel"
	"github.com/pkg/errors"
	"time"
)

func (self *tunneler) IsTerminatorValid(id string, destination string) bool {
	terminator, found := self.hostedServices.Get(id)
	if found {
		self.hostedServices.markEstablished(terminator, "validation message received")
	}
	return found
}

func (self *tunneler) Dial(params xgress.DialParams) (xt.PeerData, error) {
	destination := params.GetDestination()
	circuitId := params.GetCircuitId()

	log := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(params.GetLogContext()).
		WithField("binding", "tunnel").
		WithField("destination", destination)

	terminator, ok := self.hostedServices.Get(destination)
	if !ok {
		return nil, xgress.InvalidTerminatorError{InnerError: errors.Errorf("tunnel terminator for destination %v not found", destination)}
	}

	options, err := tunnel.AppDataToMap(circuitId.Data[uint32(edge.AppDataHeader)])
	if err != nil {
		return nil, err
	}

	//TODO: Figure out timeout
	conn, halfClose, err := terminator.context.Dial(options)
	if err != nil {
		return nil, err
	}

	log.Debugf("successful connection %v->%v for destination %v", conn.LocalAddr(), conn.RemoteAddr(), destination)

	xgConn := xgress_common.NewXgressConn(conn, halfClose, false)
	peerData := make(xt.PeerData, 3)
	if peerKey, ok := circuitId.Data[uint32(edge.PublicKeyHeader)]; ok {
		if publicKey, err := xgConn.SetupServerCrypto(peerKey); err != nil {
			return nil, err
		} else {
			peerData[uint32(edge.PublicKeyHeader)] = publicKey
		}
	}

	peerData[uint32(ctrl_msg.TerminatorLocalAddressHeader)] = []byte(conn.LocalAddr().String())
	peerData[uint32(ctrl_msg.TerminatorRemoteAddressHeader)] = []byte(conn.RemoteAddr().String())

	x := xgress.NewXgress(circuitId.Token, params.GetCtrlId(), params.GetAddress(), xgConn, xgress.Terminator, self.dialOptions.Options, params.GetCircuitTags())
	params.GetBindHandler().HandleXgressBind(x)
	x.Start()

	return peerData, nil
}

func (self *tunneler) Inspect(key string, timeout time.Duration) any {
	if key == inspect.ErtTerminatorsKey {
		return self.hostedServices.Inspect(timeout)
	}
	return nil
}
