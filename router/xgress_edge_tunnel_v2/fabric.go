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
	"github.com/cenkalti/backoff/v4"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/sdk-golang/ziti"
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
	"google.golang.org/protobuf/proto"
	"math"
	"net"
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
		peerData[edge.PublicKeyHeader] = keyPair.Public()
	}
	if len(appData) > 0 {
		peerData[edge.AppDataHeader] = appData
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

	peerKey, peerKeyFound := response.PeerData[edge.PublicKeyHeader]
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

	terminator := &tunnelTerminator{
		id:            id,
		provider:      self,
		context:       hostCtx,
		notifyCreated: make(chan struct{}, 1),
	}

	self.tunneler.terminators.Set(terminator.id, terminator)

	err := self.factory.env.GetRateLimiterPool().QueueWithTimeout(func() {
		self.establishTerminatorWithRetry(terminator)
	}, math.MaxInt64)

	if err != nil { // should only happen if router is shutting down
		self.tunneler.terminators.Remove(terminator.id)
		return nil, err
	}

	return terminator, nil
}

func (self *fabricProvider) establishTerminatorWithRetry(terminator *tunnelTerminator) {
	log := logrus.WithField("service", terminator.context.ServiceName())

	if terminator.closed.Load() {
		log.Info("not attempting to establish terminator, service not hostable")
		return
	}

	operation := func() error {
		var err error
		if !terminator.created.Load() {
			log.Info("attempting to establish terminator")
			err = self.establishTerminator(terminator)
			if err != nil && terminator.closed.Load() {
				return backoff.Permanent(err)
			}
		}
		return err
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 5 * time.Second
	expBackoff.MaxInterval = 5 * time.Minute

	if err := backoff.Retry(operation, expBackoff); err != nil {
		log.WithError(err).Error("stopping attempts to establish terminator, service not hostable")
	}
}

func (self *fabricProvider) establishTerminator(terminator *tunnelTerminator) error {
	start := time.Now().UnixMilli()
	log := pfxlog.Logger().
		WithField("routerId", self.factory.id.Token).
		WithField("service", terminator.context.ServiceName()).
		WithField("terminatorId", terminator.id)

	precedence := edge_ctrl_pb.TerminatorPrecedence_Default
	if terminator.context.ListenOptions().Precedence == ziti.PrecedenceRequired {
		precedence = edge_ctrl_pb.TerminatorPrecedence_Required
	} else if terminator.context.ListenOptions().Precedence == ziti.PrecedenceFailed {
		precedence = edge_ctrl_pb.TerminatorPrecedence_Failed
	}

	request := &edge_ctrl_pb.CreateTunnelTerminatorRequestV2{
		ServiceId:  terminator.context.ServiceId(),
		Address:    terminator.id,
		Cost:       uint32(terminator.context.ListenOptions().Cost),
		Precedence: precedence,
		InstanceId: terminator.context.ListenOptions().Identity,
		StartTime:  start,
	}

	ctrlCh := self.factory.ctrls.AnyCtrlChannel()
	if ctrlCh == nil {
		errStr := "no controller available, cannot create terminator"
		log.Error(errStr)
		return errors.New(errStr)
	}

	err := protobufs.MarshalTyped(request).WithTimeout(self.factory.DefaultRequestTimeout()).SendAndWaitForWire(ctrlCh)
	if err != nil {
		return err
	}

	if terminator.WaitForCreated(10 * time.Second) {
		return nil
	}

	// return an error to indicate that we need to check if a response has come back after the next interval,
	// and if not, re-send
	return errors.Errorf("timeout waiting for response to create terminator request for terminator %v on service %v",
		terminator.id, terminator.context.ServiceName())
}

func (self *fabricProvider) HandleTunnelResponse(msg *channel.Message, ctrlCh channel.Channel) {
	log := pfxlog.Logger().WithField("routerId", self.factory.id.Token)

	response := &edge_ctrl_pb.CreateTunnelTerminatorResponseV2{}

	if err := proto.Unmarshal(msg.Body, response); err != nil {
		log.WithError(err).Error("error unmarshalling create tunnel terminator response")
		return
	}

	log = log.WithField("terminatorId", response.TerminatorId)

	terminator, found := self.factory.tunneler.terminators.Get(response.TerminatorId)
	if !found {
		log.Error("no terminator found for id")
		return
	}

	if terminator.created.CompareAndSwap(false, true) {
		closeCallback := func() {
			if err := self.removeTerminator(terminator); err != nil {
				log.WithError(err).Error("failed to remove terminator after edge session was removed")
			}
			terminator.created.Store(false)
			go self.establishTerminatorWithRetry(terminator)
		}

		terminator.closeCallback.Store(closeCallback)

		if response.StartTime > 0 {
			elapsedTime := time.Since(time.UnixMilli(response.StartTime))
			log = log.WithField("createDuration", elapsedTime)
			self.factory.metricsRegistry.Timer("xgress_edge_tunnel.terminator.create_timer").Update(elapsedTime)
		}

		log.Info("received terminator created notification")
	} else {
		log.Info("received additional terminator created notification")
	}

	terminator.NotifyCreated()
}

func (self *fabricProvider) removeTerminator(terminator *tunnelTerminator) error {
	ctrlCh := self.factory.ctrls.AnyCtrlChannel()
	if ctrlCh == nil {
		return errors.New("no controller available, cannot remove terminator")
	}

	msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_RemoveTunnelTerminatorRequestType), []byte(terminator.id))
	responseMsg, err := msg.WithTimeout(self.factory.DefaultRequestTimeout()).SendForReply(ctrlCh)
	return xgress_common.CheckForFailureResult(responseMsg, err, edge_ctrl_pb.ContentType_RemoveTunnelTerminatorResponseType)
}

func (self *fabricProvider) updateTerminator(terminatorId string, cost *uint16, precedence *edge.Precedence) error {
	ctrlCh := self.factory.ctrls.AnyCtrlChannel()
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
	id            string
	provider      *fabricProvider
	context       tunnel.HostingContext
	created       atomic.Bool
	closeCallback concurrenz.AtomicValue[func()]
	closed        atomic.Bool
	notifyCreated chan struct{}
}

func (self *tunnelTerminator) SendHealthEvent(pass bool) error {
	return self.provider.sendHealthEvent(self.id, pass)
}

func (self *tunnelTerminator) NotifyCreated() {
	select {
	case self.notifyCreated <- struct{}{}:
	default:
	}
}

func (self *tunnelTerminator) WaitForCreated(timeout time.Duration) bool {
	if self.created.Load() {
		return true
	}
	select {
	case <-self.notifyCreated:
	case <-time.After(timeout):
	}
	return self.created.Load()
}

func (self *tunnelTerminator) Close() error {
	if self.closed.CompareAndSwap(false, true) {
		log := logrus.WithField("service", self.context.ServiceName()).
			WithField("routerId", self.provider.factory.id.Token).
			WithField("terminatorId", self.id)

		log.Debug("closing tunnel terminator context")
		self.context.OnClose()

		if cb := self.closeCallback.Load(); cb != nil {
			log.Debug("unregistering session listener for tunnel terminator")
			cb()
		}

		log.Debug("removing tunnel terminator")
		if err := self.provider.removeTerminator(self); err != nil {
			log.WithError(err).Error("error while removing tunnel terminator")
			return err
		}
		log.Info("removed tunnel terminator")
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
