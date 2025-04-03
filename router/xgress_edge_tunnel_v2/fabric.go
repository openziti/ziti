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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/secretstream/kx"
	"github.com/openziti/ziti/common/ctrl_msg"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/idgen"
	"github.com/openziti/ziti/router/posture"
	"github.com/openziti/ziti/router/xgress"
	"github.com/openziti/ziti/router/xgress_common"
	"github.com/openziti/ziti/tunnel"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func newProvider(factory *Factory, tunneler *tunneler) *fabricProvider {
	return &fabricProvider{
		factory:  factory,
		tunneler: tunneler,
	}
}

type fabricProvider struct {
	factory  *Factory
	tunneler *tunneler

	currentIdentity atomic.Pointer[rest_model.IdentityDetail]
}

func (self *fabricProvider) PrepForUse(string) {}

func (self *fabricProvider) GetCurrentIdentity() (*rest_model.IdentityDetail, error) {
	return self.currentIdentity.Load(), nil
}

func (self *fabricProvider) GetCurrentIdentityWithBackoff() (*rest_model.IdentityDetail, error) {
	return self.currentIdentity.Load(), nil
}

func (self *fabricProvider) updateIdentity(i *rest_model.IdentityDetail) {
	self.currentIdentity.Store(i)
}

func (self *fabricProvider) TunnelService(service tunnel.Service, terminatorInstanceId string, conn net.Conn, halfClose bool, appData []byte) error {
	keyPair, err := kx.NewKeyPair()
	if err != nil {
		return err
	}

	log := logrus.WithField("service", service.GetName())

	peerData := make(map[uint32][]byte)
	if service.IsEncryptionRequired() {
		peerData[uint32(edge.PublicKeyHeader)] = keyPair.Public()
	}
	if len(appData) > 0 {
		peerData[uint32(edge.AppDataHeader)] = appData
	}

	peerData[uint32(ctrl_msg.InitiatorLocalAddressHeader)] = []byte(conn.LocalAddr().String())
	peerData[uint32(ctrl_msg.InitiatorRemoteAddressHeader)] = []byte(conn.RemoteAddr().String())

	ctrlCh := self.factory.ctrls.AnyCtrlChannel()
	if ctrlCh == nil {
		errStr := "no controller available, cannot create circuit"
		log.Error(errStr)
		return errors.New(errStr)
	}

	log = log.WithField("ctrlId", ctrlCh.Id())

	rdm := self.factory.stateManager.RouterDataModel()
	if policy, err := posture.HasAccess(rdm, self.factory.routerConfig.Id.Token, service.GetId(), nil, edge_ctrl_pb.PolicyType_DialPolicy); err != nil && policy != nil {
		return fmt.Errorf("router does not have access to service '%s' (%w)", service.GetName(), err)
	}

	request := &edge_ctrl_pb.CreateTunnelCircuitV2Request{
		ServiceName:          service.GetName(),
		TerminatorInstanceId: terminatorInstanceId,
		PeerData:             peerData,
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(service.GetDialTimeout()).SendForReply(ctrlCh)

	response := &edge_ctrl_pb.CreateTunnelCircuitV2Response{}
	if err = xgress_common.GetResultOrFailure(responseMsg, err, response); err != nil {
		log.WithError(err).Warn("failed to dial fabric")
		return err
	}

	peerKey, peerKeyFound := response.PeerData[uint32(edge.PublicKeyHeader)]
	if service.IsEncryptionRequired() && !peerKeyFound {
		return errors.New("service requires encryption, but public key header not returned")
	}

	xgConn := xgress_common.NewXgressConn(conn, halfClose, false)

	if peerKeyFound {
		if err = xgConn.SetupClientCrypto(keyPair, peerKey); err != nil {
			return err
		}
	}

	x := xgress.NewXgress(response.CircuitId, ctrlCh.Id(), xgress.Address(response.Address), xgConn, xgress.Initiator, self.tunneler.listenOptions.Options, response.Tags)
	self.tunneler.bindHandler.HandleXgressBind(x)
	x.Start()

	return nil
}

func (self *fabricProvider) HostService(hostCtx tunnel.HostingContext) (tunnel.HostControl, error) {
	id := idgen.NewUUIDString()
	id = self.GetCachedTerminatorId(hostCtx.ServiceId(), id)

	terminator := &tunnelTerminator{
		id:         id,
		state:      concurrenz.AtomicValue[xgress_common.TerminatorState]{},
		provider:   self,
		context:    hostCtx,
		createTime: time.Now(),
	}
	terminator.state.Store(xgress_common.TerminatorStateEstablishing)

	self.factory.hostedServices.EstablishTerminator(terminator)

	return terminator, nil
}

func (self *fabricProvider) GetCachedTerminatorId(serviceId string, fallback string) string {
	cache := self.factory.env.GetRouterDataModel().GetTerminatorIdCache()
	result, found := cache.Get(serviceId)
	if found {
		return result
	}

	return cache.Upsert(serviceId, fallback, func(exist bool, valueInMap string, newValue string) string {
		if exist {
			return valueInMap
		}
		return newValue
	})
}

func (self *fabricProvider) updateTerminator(terminatorId string, cost *uint16, precedence *edge.Precedence) error {
	ctrlCh := self.factory.ctrls.GetModelUpdateCtrlChannel()
	if ctrlCh == nil {
		return errors.New("no controller available, cannot update terminator")
	}

	request := &edge_ctrl_pb.UpdateTunnelTerminatorRequest{
		TerminatorId: terminatorId,
	}

	if cost != nil {
		request.Cost = uint32(*cost)
		request.UpdateCost = true
	}

	if precedence != nil {
		request.Precedence = edge_ctrl_pb.TerminatorPrecedence_Default
		request.UpdatePrecedence = true
		if *precedence == edge.PrecedenceRequired {
			request.Precedence = edge_ctrl_pb.TerminatorPrecedence_Required
		} else if *precedence == edge.PrecedenceFailed {
			request.Precedence = edge_ctrl_pb.TerminatorPrecedence_Failed
		}
	}

	log := logrus.WithField("terminator", terminatorId).
		WithField("precedence", request.Precedence).
		WithField("cost", request.Cost).
		WithField("updatingPrecedence", request.UpdatePrecedence).
		WithField("updatingCost", request.UpdateCost)

	log.Debug("updating terminator")

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(self.factory.DefaultRequestTimeout()).SendForReply(ctrlCh)
	if err := xgress_common.CheckForFailureResult(responseMsg, err, edge_ctrl_pb.ContentType_UpdateTunnelTerminatorResponseType); err != nil {
		log.WithError(err).Error("terminator update failed")
		return err
	}

	log.Debug("terminator updated successfully")
	return nil
}

func (self *fabricProvider) sendHealthEvent(terminatorId string, checkPassed bool) error {
	ctrlCh := self.factory.ctrls.AnyCtrlChannel()
	if ctrlCh == nil {
		return errors.New("no controller available, cannot forward health event")
	}

	msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_TunnelHealthEventType), nil)
	msg.Headers[int32(edge_ctrl_pb.Header_TerminatorId)] = []byte(terminatorId)
	msg.PutBoolHeader(int32(edge_ctrl_pb.Header_CheckPassed), checkPassed)

	logger := logrus.WithField("terminator", terminatorId).
		WithField("checkPassed", checkPassed)
	logger.Debug("sending health event")

	if err := msg.WithTimeout(self.factory.ctrls.DefaultRequestTimeout()).Send(ctrlCh); err != nil {
		logger.WithError(err).Error("health event send failed")
	} else {
		logger.Debug("health event sent")
	}

	return nil
}

type tunnelTerminator struct {
	id string

	state             concurrenz.AtomicValue[xgress_common.TerminatorState]
	provider          *fabricProvider
	context           tunnel.HostingContext
	closed            atomic.Bool
	operationActive   atomic.Bool
	createTime        time.Time
	lastAttempt       time.Time
	rateLimitCallback rate.RateLimitControl
	lock              sync.Mutex
}

func (self *tunnelTerminator) SendHealthEvent(pass bool) error {
	return self.provider.sendHealthEvent(self.id, pass)
}

func (self *tunnelTerminator) Close() error {
	if self.closed.CompareAndSwap(false, true) {
		self.provider.factory.stateManager.RouterDataModel().GetTerminatorIdCache().Remove(self.context.ServiceId())

		log := logrus.WithField("service", self.context.ServiceName()).
			WithField("routerId", self.provider.factory.id.Token).
			WithField("terminatorId", self.id)

		self.provider.factory.hostedServices.queueRemoveTerminatorAsync(self, "close called")
		log.Info("queued tunnel terminator remove")

		log.Debug("closing tunnel terminator context")
		self.context.OnClose()
		return nil
	}
	return nil
}

func (self *tunnelTerminator) UpdateCost(cost uint16) error {
	return self.updateCostAndPrecedence(&cost, nil)
}

func (self *tunnelTerminator) UpdatePrecedence(precedence edge.Precedence) error {
	return self.updateCostAndPrecedence(nil, &precedence)
}

func (self *tunnelTerminator) UpdateCostAndPrecedence(cost uint16, precedence edge.Precedence) error {
	return self.updateCostAndPrecedence(&cost, &precedence)
}

func (self *tunnelTerminator) updateCostAndPrecedence(cost *uint16, precedence *edge.Precedence) error {
	return self.provider.updateTerminator(self.id, cost, precedence)
}

func (self *tunnelTerminator) IsEstablishing() bool {
	return self.state.Load() == xgress_common.TerminatorStateEstablishing
}

func (self *tunnelTerminator) IsDeleting() bool {
	return self.state.Load() == xgress_common.TerminatorStateDeleting
}

func (self *tunnelTerminator) setState(state xgress_common.TerminatorState, reason string) {
	if oldState := self.state.Load(); oldState != state {
		self.state.Store(state)
		pfxlog.Logger().WithField("terminatorId", self.id).
			WithField("oldState", oldState).
			WithField("newState", state).
			WithField("reason", reason).
			Info("updated state")
	}
}

func (self *tunnelTerminator) updateState(oldState, newState xgress_common.TerminatorState, reason string) bool {
	log := pfxlog.Logger().WithField("terminatorId", self.id).
		WithField("oldState", oldState).
		WithField("newState", newState).
		WithField("reason", reason)
	success := self.state.CompareAndSwap(oldState, newState)
	if success {
		log.Info("updated state")
	}
	return success
}

func (self *tunnelTerminator) SetRateLimitCallback(control rate.RateLimitControl) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.rateLimitCallback = control
}

func (self *tunnelTerminator) GetAndClearRateLimitCallback() rate.RateLimitControl {
	self.lock.Lock()
	defer self.lock.Unlock()
	result := self.rateLimitCallback
	self.rateLimitCallback = nil
	return result
}
