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
	"net"
	"sync/atomic"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/secretstream/kx"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xgress_common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func NewFabric(env env.RouterEnv, options *xgress.Options) (Fabric, error) {
	result := &fabricImpl{
		env:          env,
		options:      options,
		dialCircuits: xgress_common.NewDialCircuitRegistry(),
	}

	env.MarkRouterDataModelRequired()
	env.GetRouterDataModel().SubscribeToIdentityChanges(env.GetRouterId().Token, result, true)

	return result, nil
}

type fabricImpl struct {
	env             env.RouterEnv
	options         *xgress.Options
	currentIdentity atomic.Pointer[common.IdentityState]
	servicesByName  concurrenz.AtomicValue[map[string]*common.IdentityService]
	dialCircuits    *xgress_common.DialCircuitRegistry
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
	if eventType == common.IdentityDeletedEvent || state.Identity.Disabled {
		self.dialCircuits.CloseAll("identity deleted or disabled")
	} else if eventType == common.IdentityPostureChecksUpdatedEvent {
		// Posture checks added to a granting dial policy arrive here rather than as a service change;
		// routers cannot pass them, so re-evaluate and close circuits no longer granted. Dispatched
		// outside the data model's identity lock, so the access check cannot re-enter it.
		self.dialCircuits.RevalidateAll(self.checkDialAccess)
	} else if eventType == common.IdentityFullState {
		for _, service := range state.Services {
			pfxlog.Logger().Infof("identity %s gained access to %s", state.Identity.Name, service.GetName())
		}
	}
}

func (self *fabricImpl) NotifyBatchComplete(_ *common.RouterDataModel, _ uint64) {}

func (self *fabricImpl) NotifyServiceChange(state *common.IdentityState, oldService, newService *common.IdentityService, eventType common.ServiceEventType) {
	self.updateIdentityState(state)

	if eventType == common.ServiceAccessGainedEvent {
		pfxlog.Logger().Infof("identity %s gained access to %s", state.Identity.Name, newService.GetName())
	} else if eventType == common.ServiceAccessLostEvent {
		pfxlog.Logger().Infof("identity %s lost access to %s", state.Identity.Name, oldService.GetName())
		self.dialCircuits.CloseForService(oldService.GetId(), "service access lost")
	} else if eventType == common.ServiceUpdatedEvent && oldService != nil && oldService.IsDialAllowed() && !newService.IsDialAllowed() {
		self.dialCircuits.CloseForService(newService.GetId(), "dial access lost")
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

	ctrlCh := self.env.GetNetworkControllers().AnyChannel()
	if ctrlCh == nil {
		errStr := "no controller available, cannot create circuit"
		log.Error(errStr)
		return errors.New(errStr)
	}

	log = log.WithField("ctrlId", ctrlCh.Id())

	if err = self.checkDialAccess(service.GetId()); err != nil {
		return err
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

	xgConn := xgress_common.NewXgressConn(conn, halfClose, xgress_common.ConnTypeTunnel)

	if peerKeyFound {
		if err = xgConn.SetupClientCrypto(keyPair, peerKey); err != nil {
			return err
		}
	}

	x := xgress.NewXgress(response.CircuitId, ctrlCh.Id(), xgress.Address(response.Address), xgConn, xgress.Initiator, self.options, response.Tags)
	self.dialCircuits.PrepareTrack(x)
	self.env.GetXgressBindHandler().HandleXgressBind(x)
	self.dialCircuits.Publish(service.GetId(), x)
	x.Start()

	// Access lost between the pre-dial check and Publish is only closed by the revocation
	// notification if it saw the circuit in the registry, so re-check now that it is published.
	if err = self.checkDialAccess(service.GetId()); err != nil {
		x.Close()
		return err
	}

	return nil
}

// checkDialAccess reports whether this router currently has dial access to serviceId. Routers
// submit no posture data, so a policy carrying posture checks denies.
func (self *fabricImpl) checkDialAccess(serviceId string) error {
	return xgress_common.CheckRouterDialAccess(self.env.GetRouterDataModel(), self.env.GetRouterId().Token, serviceId)
}
