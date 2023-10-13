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
	"encoding/json"
	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/sdk-golang/ziti/sdkinfo"
	"github.com/openziti/secretstream/kx"
	"github.com/openziti/ziti/common/build"
	"github.com/openziti/ziti/common/ctrl_msg"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/router/xgress"
	"github.com/openziti/ziti/router/xgress_common"
	"github.com/openziti/ziti/tunnel"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"math"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func ToPtr[T any](in T) *T {
	return &in
}

func newProvider(factory *Factory, tunneler *tunneler) *fabricProvider {
	return &fabricProvider{
		factory:          factory,
		tunneler:         tunneler,
		apiSessionTokens: map[string]string{},
		dialSessions:     cmap.New[string](),
		bindSessions:     cmap.New[string](),
	}
}

type fabricProvider struct {
	factory  *Factory
	tunneler *tunneler

	apiSessionLock   sync.Mutex
	apiSessionTokens map[string]string
	currentIdentity  *rest_model.IdentityDetail

	dialSessions cmap.ConcurrentMap[string, string]
	bindSessions cmap.ConcurrentMap[string, string]
}

func (self *fabricProvider) getDialSession(serviceName string) string {
	sessionId, _ := self.dialSessions.Get(serviceName)
	return sessionId
}

func (self *fabricProvider) getBindSession(serviceName string) string {
	sessionId, _ := self.bindSessions.Get(serviceName)
	return sessionId
}

func (self *fabricProvider) updateApiSession(ctrlId string, resp *edge_ctrl_pb.CreateApiSessionResponse) bool {
	self.apiSessionLock.Lock()
	defer self.apiSessionLock.Unlock()

	currentToken := self.apiSessionTokens[ctrlId]
	if currentToken == resp.Token {
		return false
	}

	self.tunneler.stateManager.RemoveConnectedApiSession(currentToken)

	self.apiSessionTokens[ctrlId] = resp.Token
	self.currentIdentity = &rest_model.IdentityDetail{
		BaseEntity: rest_model.BaseEntity{
			ID: &resp.IdentityId,
		},
		Name:                      &resp.IdentityName,
		DefaultHostingPrecedence:  rest_model.TerminatorPrecedence(strings.ToLower(resp.DefaultHostingPrecedence.String())),
		DefaultHostingCost:        ToPtr(rest_model.TerminatorCost(int64(resp.DefaultHostingCost))),
		AppData:                   &rest_model.Tags{},
		ServiceHostingPrecedences: rest_model.TerminatorPrecedenceMap{},
		ServiceHostingCosts:       rest_model.TerminatorCostMap{},
	}

	for k, v := range resp.ServicePrecedences {
		self.currentIdentity.ServiceHostingPrecedences[k] = v.GetZitiLabel()
	}

	for k, v := range resp.ServiceCosts {
		self.currentIdentity.ServiceHostingCosts[k] = ToPtr(rest_model.TerminatorCost(v))
	}

	if resp.AppDataJson != "" {
		decoder := json.NewDecoder(strings.NewReader(resp.AppDataJson))
		err := decoder.Decode(&self.currentIdentity.AppData)
		if err != nil {
			logrus.WithError(err).Errorf("failed to decode appDataJson: '%v'", resp.AppDataJson)
		}
	}

	self.tunneler.stateManager.AddConnectedApiSession(resp.Token)
	return true
}

func (self *fabricProvider) authenticate() error {
	envInfo, _ := sdkinfo.GetSdkInfo()
	buildInfo := build.GetBuildInfo()
	osVersion := "unknown"
	osRelease := "unknown"

	if envInfo.OsVersion != "" {
		osVersion = envInfo.OsVersion
	}

	if envInfo.OsRelease != "" {
		osRelease = envInfo.OsRelease
	}

	request := &edge_ctrl_pb.CreateApiSessionRequest{
		EnvInfo: &edge_ctrl_pb.EnvInfo{
			Os:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			OsVersion: osVersion,
			OsRelease: osRelease,
		},
		SdkInfo: &edge_ctrl_pb.SdkInfo{
			AppId:      "ziti-router",
			AppVersion: buildInfo.Version(),
			Branch:     buildInfo.Branch(),
			Revision:   buildInfo.Revision(),
			Type:       "ziti-router:tunnel",
			Version:    buildInfo.Version(),
		},
		ConfigTypes: []string{
			"f2dd2df0-9c04-4b84-a91e-71437ac229f1", // client v1
			"cea49285-6c07-42cf-9f52-09a9b115c783", // server v1
			"g7cIWbcGg",                            // intercept.v1
			"NH5p4FpGR",                            // host.v1
			"host.v2",                              // host.v2
		},
	}

	ctrlMap := self.factory.ctrls.GetAll()
	ctrlChMap := map[string]channel.Channel{}
	for k, v := range ctrlMap {
		ctrlChMap[k] = v.Channel()
	}
	if len(ctrlChMap) == 0 {
		ctrlCh := self.factory.ctrls.AnyValidCtrlChannel()
		ctrlChMap[ctrlCh.Id()] = ctrlCh
	}
	results := newAuthResults(len(ctrlChMap))
	for k, v := range ctrlChMap {
		ctrlId := k
		ctrlCh := v
		go func() {
			respMsg, err := protobufs.MarshalTyped(request).WithTimeout(30 * time.Second).SendForReply(ctrlCh)

			resp := &edge_ctrl_pb.CreateApiSessionResponse{}
			if err = xgress_common.GetResultOrFailure(respMsg, err, resp); err != nil {
				results.Error(err)
				pfxlog.Logger().WithError(err).WithField("ctrlId", ctrlId).Error("failed to authenticate")
				return
			}

			self.updateApiSession(ctrlId, resp)
			results.Success()
		}()
	}

	return results.GetResults()
}

func (self *fabricProvider) PrepForUse(string) {}

func (self *fabricProvider) GetCurrentIdentity() (*rest_model.IdentityDetail, error) {
	return self.currentIdentity, nil
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

	sessionId := self.getDialSession(service.GetName())
	request := &edge_ctrl_pb.CreateCircuitForServiceRequest{
		SessionId:            sessionId,
		ServiceName:          service.GetName(),
		TerminatorInstanceId: terminatorInstanceId,
		PeerData:             peerData,
	}

	ctrlCh := self.factory.ctrls.AnyCtrlChannel()
	if ctrlCh == nil {
		errStr := "no controller available, cannot create circuit"
		log.Error(errStr)
		return errors.New(errStr)
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(service.GetDialTimeout()).SendForReply(ctrlCh)

	response := &edge_ctrl_pb.CreateCircuitForServiceResponse{}
	if err = xgress_common.GetResultOrFailure(responseMsg, err, response); err != nil {
		log.WithError(err).Warn("failed to dial fabric")
		return err
	}

	if response.ApiSession != nil {
		if self.updateApiSession(ctrlCh.Id(), response.ApiSession) {
			log.WithField("apiSessionId", response.ApiSession.SessionId).Info("received new apiSession")
		}
	}

	if response.Session != nil && response.Session.SessionId != sessionId {
		log.WithField("sessionId", response.Session.SessionId).Info("received new session")
		self.dialSessions.Set(service.GetName(), response.Session.SessionId)
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

	cleanupCallback := self.tunneler.stateManager.AddEdgeSessionRemovedListener(response.Session.Token, func(token string) {
		if err = conn.Close(); err != nil {
			log.WithError(err).Error("failed to close external conn when session closed")
		}
	})

	x := xgress.NewXgress(response.CircuitId, ctrlCh.Id(), xgress.Address(response.Address), xgConn, xgress.Initiator, self.tunneler.listenOptions.Options, response.Tags)
	self.tunneler.bindHandler.HandleXgressBind(x)
	x.AddCloseHandler(xgress.CloseHandlerF(func(x *xgress.Xgress) { cleanupCallback() }))
	x.Start()

	return nil
}

func (self *fabricProvider) HostService(hostCtx tunnel.HostingContext) (tunnel.HostControl, error) {
	id := uuid.NewString()

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

	sessionId := self.getBindSession(terminator.context.ServiceName())
	request := &edge_ctrl_pb.CreateTunnelTerminatorRequest{
		ServiceName: terminator.context.ServiceName(),
		SessionId:   sessionId,
		Address:     terminator.id,
		Cost:        uint32(terminator.context.ListenOptions().Cost),
		Precedence:  precedence,
		InstanceId:  terminator.context.ListenOptions().Identity,
		StartTime:   start,
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

	response := &edge_ctrl_pb.CreateTunnelTerminatorResponse{}

	if err := proto.Unmarshal(msg.Body, response); err != nil {
		log.WithError(err).Error("error unmarshalling create tunnel terminator response")
		return
	}

	if response.ApiSession != nil {
		if self.updateApiSession(ctrlCh.Id(), response.ApiSession) {
			log.WithField("apiSessionId", response.ApiSession.SessionId).Info("received new api-session")
		}
	}

	log = log.WithField("terminatorId", response.TerminatorId)

	terminator, found := self.factory.tunneler.terminators.Get(response.TerminatorId)
	if !found {
		log.Error("no terminator found for id")
		return
	}

	sessionId, _ := self.bindSessions.Get(terminator.context.ServiceName())
	if response.Session != nil && response.Session.SessionId != sessionId {
		log.WithField("sessionId", response.Session.SessionId).Info("received new session")
		self.bindSessions.Set(terminator.context.ServiceName(), response.Session.SessionId)
	}

	if terminator.created.CompareAndSwap(false, true) {
		// TODO: How do we make sure we don't get dups here. Probably not a big problem and this will need to be refactored for JWT sessions in any case
		closeCallback := self.tunneler.stateManager.AddEdgeSessionRemovedListener(response.Session.Token, func(token string) {
			if err := self.removeTerminator(terminator); err != nil {
				log.WithError(err).Error("failed to remove terminator after edge session was removed")
			}
			terminator.created.Store(false)
			go self.establishTerminatorWithRetry(terminator)
		})

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

func (self *fabricProvider) requestServiceList(ctrlCh channel.Channel, lastUpdateToken []byte) {
	msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_ListServicesRequestType), lastUpdateToken)
	if err := ctrlCh.Send(msg); err != nil {
		logrus.WithError(err).Error("failed to send service list request to controller")
	}
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

func newAuthResults(count int) *authResults {
	return &authResults{
		ctrlCount: count,
		okCh:      make(chan interface{}, 1),
		errCh:     make(chan error, count),
	}
}

type authResults struct {
	ctrlCount int
	okCh      chan interface{}
	errCh     chan error
}

func (self *authResults) Success() {
	select {
	case self.okCh <- nil:
	default:
	}
}

func (self *authResults) Error(err error) {
	self.errCh <- err
}

func (self *authResults) GetResults() error {
	var errList []error
	for {
		select {
		case <-self.okCh: // if any succeeded, return nil
			return nil
		case err := <-self.errCh:
			errList = append(errList, err)
			if len(errList) == self.ctrlCount {
				return errorz.MultipleErrors(errList)
			}
		}
	}
}
