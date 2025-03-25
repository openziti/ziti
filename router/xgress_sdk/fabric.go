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

package xgress_sdk

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/secretstream/kx"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/ctrl_msg"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/posture"
	"github.com/openziti/ziti/router/xgress"
	"github.com/openziti/ziti/router/xgress_common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net"
	"sync/atomic"
)

func NewFabric(env env.RouterEnv, options *xgress.Options) (Fabric, error) {
	result := &fabricImpl{
		env:     env,
		options: options,
	}

	if err := env.GetRouterDataModel().SubscribeToIdentityChanges(env.GetRouterId().Token, result, true); err != nil {
		return nil, err
	}

	return result, nil
}

type fabricImpl struct {
	env             env.RouterEnv
	options         *xgress.Options
	currentIdentity atomic.Pointer[common.IdentityState]
	servicesByName  concurrenz.AtomicValue[map[string]*common.IdentityService]
}

func (self *fabricImpl) updateIdentityState(state *common.IdentityState) {
	self.currentIdentity.Store(state)
	servicesByName := map[string]*common.IdentityService{}
	if state != nil {
		for _, v := range state.Services {
			servicesByName[v.Service.Name] = v
		}
	}
	self.servicesByName.Store(servicesByName)
}

func (self *fabricImpl) NotifyIdentityEvent(state *common.IdentityState, eventType common.IdentityEventType) {
	self.updateIdentityState(state)
	pfxlog.Logger().Infof("identity %s event %s", state.Identity.Name, eventType.String())
	if eventType == common.EventFullState {
		for _, service := range state.Services {
			pfxlog.Logger().Infof("identity %s gained access to %s", state.Identity.Name, service.GetName())
		}
	}
}

func (self *fabricImpl) NotifyServiceChange(state *common.IdentityState, service *common.IdentityService, eventType common.ServiceEventType) {
	self.updateIdentityState(state)

	if eventType == common.EventAccessGained {
		pfxlog.Logger().Infof("identity %s gained access to %s", state.Identity.Name, service.GetName())
	} else if eventType == common.EventAccessRemoved {
		pfxlog.Logger().Infof("identity %s lost access to %s", state.Identity.Name, service.GetName())
	}
}

func (self *fabricImpl) TunnelWithOptions(serviceName string, options *ziti.DialOptions, conn net.Conn, halfClose bool) error {
	service := self.servicesByName.Load()[serviceName]
	if service == nil {
		return fmt.Errorf("service %s not found", serviceName)
	}

	keyPair, err := kx.NewKeyPair()
	if err != nil {
		return err
	}

	log := logrus.WithField("service", serviceName)

	peerData := make(map[uint32][]byte)
	if service.Service.EncryptionRequired {
		peerData[uint32(edge.PublicKeyHeader)] = keyPair.Public()
	}

	if len(options.AppData) > 0 {
		peerData[uint32(edge.AppDataHeader)] = options.AppData
	}

	peerData[uint32(ctrl_msg.InitiatorLocalAddressHeader)] = []byte(conn.LocalAddr().String())
	peerData[uint32(ctrl_msg.InitiatorRemoteAddressHeader)] = []byte(conn.RemoteAddr().String())

	ctrlCh := self.env.GetNetworkControllers().AnyCtrlChannel()
	if ctrlCh == nil {
		errStr := "no controller available, cannot create circuit"
		log.Error(errStr)
		return errors.New(errStr)
	}

	log = log.WithField("ctrlId", ctrlCh.Id())

	rdm := self.env.GetRouterDataModel()
	if policy, err := posture.HasAccess(rdm, self.env.GetRouterId().Token, service.GetId(), nil, edge_ctrl_pb.PolicyType_DialPolicy); err != nil && policy != nil {
		return fmt.Errorf("router does not have access to service '%s' (%w)", serviceName, err)
	}

	request := &edge_ctrl_pb.CreateTunnelCircuitV2Request{
		ServiceName:          serviceName,
		TerminatorInstanceId: options.Identity,
		PeerData:             peerData,
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(options.GetConnectTimeout()).SendForReply(ctrlCh)

	response := &edge_ctrl_pb.CreateTunnelCircuitV2Response{}
	if err = xgress_common.GetResultOrFailure(responseMsg, err, response); err != nil {
		log.WithError(err).Warn("failed to dial fabric")
		return err
	}

	peerKey, peerKeyFound := response.PeerData[uint32(edge.PublicKeyHeader)]
	if service.Service.EncryptionRequired && !peerKeyFound {
		return errors.New("service requires encryption, but public key header not returned")
	}

	xgConn := xgress_common.NewXgressConn(conn, halfClose, false)

	if peerKeyFound {
		if err = xgConn.SetupClientCrypto(keyPair, peerKey); err != nil {
			return err
		}
	}

	x := xgress.NewXgress(response.CircuitId, ctrlCh.Id(), xgress.Address(response.Address), xgConn, xgress.Initiator, self.options, response.Tags)
	self.env.GetXgressBindHandler().HandleXgressBind(x)
	x.Start()

	return nil
}
