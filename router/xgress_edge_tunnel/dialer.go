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

package xgress_edge_tunnel

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/router/xgress_common"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
)

func (self *tunneler) IsTerminatorValid(_ string, destination string) bool {
	_, found := self.terminators.Get(destination)
	return found
}

func (self *tunneler) Dial(destination string, circuitId *identity.TokenId, address xgress.Address, bindHandler xgress.BindHandler, ctx logcontext.Context) (xt.PeerData, error) {
	log := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(ctx).
		WithField("binding", "edge").
		WithField("destination", destination)

	val, ok := self.terminators.Get(destination)
	if !ok {
		return nil, xgress.InvalidTerminatorError{InnerError: errors.Errorf("tunnel terminator for destination %v not found", destination)}
	}
	terminator := val.(*tunnelTerminator)

	options, err := tunnel.AppDataToMap(circuitId.Data[edge.AppDataHeader])
	if err != nil {
		return nil, err
	}

	conn, halfClose, err := terminator.context.Dial(options)
	if err != nil {
		return nil, err
	}

	log.Infof("successful connection %v->%v for destination %v", conn.LocalAddr(), conn.RemoteAddr(), destination)

	xgConn := xgress_common.NewXgressConn(conn, halfClose, false)
	peerData := make(xt.PeerData, 1)
	if peerKey, ok := circuitId.Data[edge.PublicKeyHeader]; ok {
		if publicKey, err := xgConn.SetupServerCrypto(peerKey); err != nil {
			return nil, err
		} else {
			peerData[edge.PublicKeyHeader] = publicKey
		}
	}

	peerData[uint32(ctrl_pb.ContentType_TerminatorLocalAddressHeader)] = []byte(conn.LocalAddr().String())

	x := xgress.NewXgress(circuitId, address, xgConn, xgress.Terminator, self.dialOptions.Options)
	bindHandler.HandleXgressBind(x)
	x.Start()

	return peerData, nil
}
