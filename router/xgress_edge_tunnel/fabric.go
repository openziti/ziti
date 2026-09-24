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

package xgress_edge_tunnel

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/secretstream/kx"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/posture"
	"github.com/openziti/ziti/v2/router/xgress_common"
	"github.com/openziti/ziti/v2/tunnel"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func NewTunnelFabricProvider(routerEnv env.RouterEnv, hostedServicesRegistry *HostedServiceRegistry) TunnelFabricProvider {
	return &fabricProvider{
		env:            routerEnv,
		hostedServices: hostedServicesRegistry,
		bindHandler:    routerEnv.GetXgressBindHandler(),
	}
}

type fabricProvider struct {
	env            env.RouterEnv
	hostedServices *HostedServiceRegistry
	options        *xgress.Options
	bindHandler    xgress.BindHandler

	currentIdentity atomic.Pointer[rest_model.IdentityDetail]
}

func (self *fabricProvider) PrepForUse(string) {}

func (self *fabricProvider) GetCurrentIdentity() (*rest_model.IdentityDetail, error) {
	return self.currentIdentity.Load(), nil
}

func (self *fabricProvider) GetCurrentIdentityWithBackoff() (*rest_model.IdentityDetail, error) {
	return self.currentIdentity.Load(), nil
}

func (self *fabricProvider) UpdateIdentity(i *rest_model.IdentityDetail) {
	self.currentIdentity.Store(i)
}

func (self *fabricProvider) SetXgressOptions(options *xgress.Options) {
	self.options = options
}

func (self *fabricProvider) TunnelService(service tunnel.Service, terminatorInstanceId string, conn net.Conn, halfClose bool, appData []byte) error {
	keyPair, err := kx.NewKeyPair()
	if err != nil {
		return err
	}

	log := logrus.WithField("service", service.GetName()).WithField("src", conn.RemoteAddr().String())

	peerData := make(map[uint32][]byte)
	if service.IsEncryptionRequired() {
		peerData[uint32(edge.PublicKeyHeader)] = keyPair.Public()
	}
	if len(appData) > 0 {
		peerData[uint32(edge.AppDataHeader)] = appData
	}

	peerData[uint32(ctrl_msg.InitiatorLocalAddressHeader)] = []byte(conn.LocalAddr().String())
	peerData[uint32(ctrl_msg.InitiatorRemoteAddressHeader)] = []byte(conn.RemoteAddr().String())

	ctrlCh := self.env.GetNetworkControllers().AnyCtrlChannel()
	if ctrlCh == nil {
		errStr := "no controller available, cannot create circuit"
		log.Error(errStr)
		return errors.New(errStr)
	}

	log = log.WithField("ctrlId", ctrlCh.PeerId())

	rdm := self.env.GetRouterDataModel()
	if policy, err := posture.HasAccess(rdm, self.env.GetRouterId().Token, service.GetId(), nil, edge_ctrl_pb.PolicyType_DialPolicy); err != nil && policy != nil {
		return fmt.Errorf("router does not have access to service '%s' (%w)", service.GetName(), err)
	}

	request := &edge_ctrl_pb.CreateTunnelCircuitV2Request{
		ServiceName:          service.GetName(),
		TerminatorInstanceId: terminatorInstanceId,
		PeerData:             peerData,
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(service.GetDialTimeout()).SendForReply(ctrlCh.GetHighPrioritySender())

	response := &edge_ctrl_pb.CreateTunnelCircuitV2Response{}
	if err = xgress_common.GetResultOrFailure(responseMsg, err, response); err != nil {
		log.WithError(err).Warn("failed to dial fabric")
		return err
	}

	log.WithField("circuitId", response.CircuitId).Debug("circuit established")

	peerKey, peerKeyFound := response.PeerData[uint32(edge.PublicKeyHeader)]
	if service.IsEncryptionRequired() && !peerKeyFound {
		return errors.New("service requires encryption, but public key header not returned")
	}

	xgConn := xgress_common.NewXgressConn(conn, halfClose, xgress_common.ConnTypeTunnel)

	if peerKeyFound {
		if err = xgConn.SetupClientCrypto(keyPair, peerKey); err != nil {
			return err
		}
	}

	x := xgress.NewXgress(response.CircuitId, ctrlCh.PeerId(), xgress.Address(response.Address), xgConn, xgress.Initiator, self.options, response.Tags)
	self.bindHandler.HandleXgressBind(x)
	x.Start()

	return nil
}

func (self *fabricProvider) HostService(hostCtx tunnel.HostingContext) (tunnel.HostControl, error) {
	id := idgen.MustNewUUIDString()
	id = self.GetCachedTerminatorId(hostCtx.GetTerminatorIdCacheKey(), id)

	terminator := &tunnelTerminator{
		id:         id,
		state:      concurrenz.AtomicValue[xgress_common.TerminatorState]{},
		provider:   self,
		context:    hostCtx,
		createTime: time.Now(),
	}
	terminator.state.Store(xgress_common.TerminatorStateEstablishing)

	self.hostedServices.EstablishTerminator(terminator)

	return terminator, nil
}

func (self *fabricProvider) GetCachedTerminatorId(terminatorKey string, fallback string) string {
	cache := self.env.GetRouterDataModel().GetTerminatorIdCache()
	result, found := cache.Get(terminatorKey)
	if found {
		return result
	}

	return cache.Upsert(terminatorKey, fallback, func(exist bool, valueInMap string, newValue string) string {
		if exist {
			return valueInMap
		}
		return newValue
	})
}

func (self *fabricProvider) updateTerminator(terminatorId string, cost *uint16, precedence *edge.Precedence) error {
	ctrlCh := self.env.GetNetworkControllers().GetModelUpdateCtrlChannel()
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

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(self.env.DefaultRequestTimeout()).SendForReply(ctrlCh)
	if err := xgress_common.CheckForFailureResult(responseMsg, err, edge_ctrl_pb.ContentType_UpdateTunnelTerminatorResponseType); err != nil {
		log.WithError(err).Error("terminator update failed")
		return err
	}

	log.Debug("terminator updated successfully")
	return nil
}

func (self *fabricProvider) sendHealthEvent(terminatorId string, checkPassed bool) error {
	ctrlCh := self.env.GetNetworkControllers().AnyCtrlChannel()
	if ctrlCh == nil {
		return errors.New("no controller available, cannot forward health event")
	}

	msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_TunnelHealthEventType), nil)
	msg.Headers[int32(edge_ctrl_pb.Header_TerminatorId)] = []byte(terminatorId)
	msg.PutBoolHeader(int32(edge_ctrl_pb.Header_CheckPassed), checkPassed)

	logger := logrus.WithField("terminator", terminatorId).
		WithField("checkPassed", checkPassed)
	logger.Debug("sending health event")

	if err := msg.WithTimeout(self.env.GetNetworkControllers().DefaultRequestTimeout()).Send(ctrlCh.GetDefaultSender()); err != nil {
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
	createRequest     *createRequestId // guarded by lock; nil when no create is awaiting a response
}

// createRequestId identifies a create terminator request by the control channel it went out on and
// the message sequence assigned to it. Every attempt for a terminator carries the same terminator id,
// so the id alone cannot tell a reply to the outstanding attempt from a reply to a superseded one.
type createRequestId struct {
	ctrlId   string
	sequence int32
}

// clearCreateRequest drops any outstanding create attempt, leaving nothing in flight. Callers invoke
// it before sending a create, so a reply to the attempt being superseded can no longer report the
// terminator as idle, and to roll back an attempt whose send failed after its sequence was assigned.
func (self *tunnelTerminator) clearCreateRequest() {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.createRequest = nil
}

// noteCreateRequestSent records the create request just sent to ctrlId as sequence, making it the
// attempt that a response must match to be treated as answering this terminator's create.
func (self *tunnelTerminator) noteCreateRequestSent(ctrlId string, sequence int32) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.createRequest = &createRequestId{ctrlId: ctrlId, sequence: sequence}
}

// resolveCreateRequest reports whether a response arriving on ctrlId in reply to replyFor answers the
// outstanding create attempt, clearing that attempt if it does. A response belonging to a superseded
// attempt returns false and leaves the outstanding attempt in place.
func (self *tunnelTerminator) resolveCreateRequest(ctrlId string, replyFor int32) bool {
	self.lock.Lock()
	defer self.lock.Unlock()
	if self.createRequest == nil || *self.createRequest != (createRequestId{ctrlId: ctrlId, sequence: replyFor}) {
		return false
	}
	self.createRequest = nil
	return true
}

// hasOutstandingCreate reports whether a create request is still awaiting a response.
func (self *tunnelTerminator) hasOutstandingCreate() bool {
	self.lock.Lock()
	defer self.lock.Unlock()
	return self.createRequest != nil
}

// endOperationIfIdle clears the in-flight marker unless a create is still awaiting a response. The
// marker is what holds a queued delete back from racing a live create, so only an outcome that leaves
// nothing outstanding may clear it.
func (self *tunnelTerminator) endOperationIfIdle() {
	self.lock.Lock()
	defer self.lock.Unlock()
	if self.createRequest == nil {
		self.operationActive.Store(false)
	}
}

func (self *tunnelTerminator) SendHealthEvent(pass bool) error {
	return self.provider.sendHealthEvent(self.id, pass)
}

func (self *tunnelTerminator) Close() error {
	if self.closed.CompareAndSwap(false, true) {
		self.provider.env.GetRouterDataModel().GetTerminatorIdCache().Remove(self.context.GetTerminatorIdCacheKey())

		log := logrus.WithField("service", self.context.ServiceName()).
			WithField("routerId", self.provider.env.GetRouterId().Token).
			WithField("terminatorId", self.id)

		self.provider.hostedServices.queueRemoveTerminatorAsync(self, "close called")
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

// replaceRateLimitCallback stores control as the rate-limit control for a new establishment attempt.
// If a control from a prior attempt is still outstanding, that attempt exceeded EstablishmentTimeout
// without completing, so its control is resolved with Backoff (signaling congestion and reclaiming its
// slot) rather than being orphaned and left for the limiter's internal timeout to clean up.
func (self *tunnelTerminator) replaceRateLimitCallback(control rate.RateLimitControl) {
	self.lock.Lock()
	previous := self.rateLimitCallback
	self.rateLimitCallback = control
	self.lock.Unlock()

	if previous != nil {
		previous.Backoff()
	}
}

// resolveRateLimitCallback resolves the terminator's outstanding rate-limit control based on how long
// establishment took. An establishment that completed within EstablishmentTimeout reports Success,
// growing the limiter window; one that took at least EstablishmentTimeout reports Backoff, signaling
// congestion so the window shrinks. It is a no-op if no control is outstanding.
func (self *tunnelTerminator) resolveRateLimitCallback(latency time.Duration) {
	if control := self.GetAndClearRateLimitCallback(); control != nil {
		if latency >= xgress_common.EstablishmentTimeout {
			control.Backoff()
		} else {
			control.Success()
		}
	}
}

func (self *tunnelTerminator) GetAndClearRateLimitCallback() rate.RateLimitControl {
	self.lock.Lock()
	defer self.lock.Unlock()
	result := self.rateLimitCallback
	self.rateLimitCallback = nil
	return result
}
