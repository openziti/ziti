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
	"encoding/json"
	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/secretstream/kx"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/edge/router/xgress_common"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/fabric/build"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/sdk-golang/ziti/sdkinfo"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/sirupsen/logrus"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"
)

func newProvider(factory *Factory, tunneler *tunneler) *fabricProvider {
	return &fabricProvider{
		factory:      factory,
		tunneler:     tunneler,
		dialSessions: cmap.New(),
		bindSessions: cmap.New(),
	}
}

type fabricProvider struct {
	factory  *Factory
	tunneler *tunneler

	apiSessionLock  sync.Mutex
	apiSessionToken string
	currentIdentity *edge.CurrentIdentity

	dialSessions cmap.ConcurrentMap
	bindSessions cmap.ConcurrentMap
}

func (self *fabricProvider) getDialSession(serviceName string) string {
	sessionId, _ := self.dialSessions.Get(serviceName)
	if sessionId != nil {
		return sessionId.(string)
	}
	return ""
}

func (self *fabricProvider) getBindSession(serviceName string) string {
	sessionId, _ := self.bindSessions.Get(serviceName)
	if sessionId != nil {
		return sessionId.(string)
	}
	return ""
}

func (self *fabricProvider) updateApiSession(resp *edge_ctrl_pb.CreateApiSessionResponse) {
	self.apiSessionLock.Lock()
	defer self.apiSessionLock.Unlock()

	self.tunneler.stateManager.RemoveConnectedApiSession(self.apiSessionToken)

	self.apiSessionToken = resp.Token
	self.currentIdentity = &edge.CurrentIdentity{
		Id:                        resp.IdentityId,
		Name:                      resp.IdentityName,
		DefaultHostingPrecedence:  strings.ToLower(resp.DefaultHostingPrecedence.String()),
		DefaultHostingCost:        uint16(resp.DefaultHostingCost),
		AppData:                   map[string]interface{}{},
		ServiceHostingPrecedences: map[string]interface{}{},
		ServiceHostingCosts:       map[string]interface{}{},
	}

	for k, v := range resp.ServicePrecedences {
		self.currentIdentity.ServiceHostingPrecedences[k] = v.GetZitiLabel()
	}

	for k, v := range resp.ServiceCosts {
		self.currentIdentity.ServiceHostingCosts[k] = float64(v)
	}

	if resp.AppDataJson != "" {
		decoder := json.NewDecoder(strings.NewReader(resp.AppDataJson))
		err := decoder.Decode(&self.currentIdentity.AppData)
		if err != nil {
			logrus.WithError(err).Errorf("failed to decode appDataJson: '%v'", resp.AppDataJson)
		}
	}

	self.tunneler.stateManager.AddConnectedApiSession(self.apiSessionToken)
}

func (self *fabricProvider) authenticate() error {
	info := sdkinfo.GetSdkInfo()
	buildInfo := build.GetBuildInfo()
	osVersion := "unknown"
	osRelease := "unknown"

	if val, ok := info["osVersion"]; ok {
		if valStr, ok := val.(string); ok {
			osVersion = valStr
		}
	}

	if val, ok := info["osRelease"]; ok {
		if valStr, ok := val.(string); ok {
			osRelease = valStr
		}
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

	respMsg, err := self.factory.Channel().SendForReply(request, 30*time.Second)

	resp := &edge_ctrl_pb.CreateApiSessionResponse{}
	if err = xgress_common.GetResultOrFailure(respMsg, err, resp); err != nil {
		return err
	}

	self.updateApiSession(resp)

	return nil
}

func (self *fabricProvider) PrepForUse(string) {}

func (self *fabricProvider) GetCurrentIdentity() (*edge.CurrentIdentity, error) {
	return self.currentIdentity, nil
}

func (self *fabricProvider) TunnelService(service tunnel.Service, terminatorIdentity string, conn net.Conn, halfClose bool, appData []byte) error {
	keyPair, err := kx.NewKeyPair()
	if err != nil {
		return err
	}

	log := logrus.WithField("service", service.GetName())

	peerData := make(map[uint32][]byte)
	peerData[edge.PublicKeyHeader] = keyPair.Public()
	if len(appData) > 0 {
		peerData[edge.AppDataHeader] = appData
	}

	sessionId := self.getDialSession(service.GetName())
	request := &edge_ctrl_pb.CreateCircuitForServiceRequest{
		SessionId:          sessionId,
		ServiceName:        service.GetName(),
		TerminatorIdentity: terminatorIdentity,
		PeerData:           peerData,
	}

	responseMsg, err := self.factory.Channel().SendForReply(request, service.GetDialTimeout())

	response := &edge_ctrl_pb.CreateCircuitForServiceResponse{}
	if err = xgress_common.GetResultOrFailure(responseMsg, err, response); err != nil {
		log.Warn("failed to dial fabric")
		return err
	}

	if response.ApiSession != nil {
		self.updateApiSession(response.ApiSession)
	}

	if response.Session != nil && response.Session.SessionId != sessionId {
		self.dialSessions.Set(service.GetName(), response.Session.SessionId)
	}

	xgConn := xgress_common.NewXgressConn(conn, halfClose, false)

	if peerKey, ok := response.PeerData[edge.PublicKeyHeader]; ok {
		if err := xgConn.SetupClientCrypto(keyPair, peerKey); err != nil {
			return err
		}
	}

	cleanupCallback := self.tunneler.stateManager.AddEdgeSessionRemovedListener(response.Session.Token, func(token string) {
		if err = conn.Close(); err != nil {
			log.WithError(err).Error("failed to close external conn when session closed")
		}
	})

	x := xgress.NewXgress(&identity.TokenId{Token: response.CircuitId}, xgress.Address(response.Address), xgConn, xgress.Initiator, self.tunneler.listenOptions.Options)
	self.tunneler.bindHandler.HandleXgressBind(x)
	x.AddCloseHandler(xgress.CloseHandlerF(func(x *xgress.Xgress) { cleanupCallback() }))
	x.Start()

	return nil
}

func (self *fabricProvider) HostService(hostCtx tunnel.HostingContext) (tunnel.HostControl, error) {
	terminator := &tunnelTerminator{
		provider: self,
		context:  hostCtx,
	}

	go self.establishTerminatorWithRetry(terminator)

	return terminator, nil
}

func (self *fabricProvider) establishTerminatorWithRetry(terminator *tunnelTerminator) {
	logger := logrus.WithField("service", terminator.context.ServiceName())

	if terminator.closed.Get() {
		logger.Info("not attempting to establish terminator, service not hostable")
		return
	}

	operation := func() error {
		logger.Info("attempting to establish terminator")
		err := self.establishTerminator(terminator)
		if err != nil && terminator.closed.Get() {
			return backoff.Permanent(err)
		}
		return err
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxInterval = 1 * time.Minute

	if err := backoff.Retry(operation, expBackoff); err != nil {
		logger.WithError(err).Error("stopping attempts to establish terminator, service not hostable")
	}
}

func (self *fabricProvider) establishTerminator(terminator *tunnelTerminator) error {
	terminator.address = uuid.NewString() // grab new id each time we retry

	logger := pfxlog.Logger().
		WithField("routerId", self.factory.id).
		WithField("service", terminator.context.ServiceName()).
		WithField("address", terminator.address)

	keyPair, err := kx.NewKeyPair()
	if err != nil {
		return err
	}

	hostData := make(map[uint32][]byte)
	hostData[edge.PublicKeyHeader] = keyPair.Public()

	precedence := edge_ctrl_pb.TerminatorPrecedence_Default
	if terminator.context.ListenOptions().Precedence == edge.PrecedenceRequired {
		precedence = edge_ctrl_pb.TerminatorPrecedence_Required
	} else if terminator.context.ListenOptions().Precedence == edge.PrecedenceFailed {
		precedence = edge_ctrl_pb.TerminatorPrecedence_Failed
	}

	self.tunneler.terminators.Set(terminator.address, terminator)

	sessionId := self.getBindSession(terminator.context.ServiceName())
	request := &edge_ctrl_pb.CreateTunnelTerminatorRequest{
		ServiceName: terminator.context.ServiceName(),
		SessionId:   sessionId,
		Address:     terminator.address,
		PeerData:    hostData,
		Cost:        uint32(terminator.context.ListenOptions().Cost),
		Precedence:  precedence,
		Identity:    terminator.context.ListenOptions().Identity,
	}

	request.GetContentType()

	response := &edge_ctrl_pb.CreateTunnelTerminatorResponse{}
	responseMsg, err := self.factory.Channel().SendForReply(request, self.factory.DefaultRequestTimeout())
	if err = xgress_common.GetResultOrFailure(responseMsg, err, response); err != nil {
		logger.WithError(err).Error("error creating terminator")
		return err
	}

	if response.ApiSession != nil {
		self.updateApiSession(response.ApiSession)
	}

	if response.Session != nil && response.Session.SessionId != sessionId {
		self.bindSessions.Set(terminator.context.ServiceName(), response.Session.SessionId)
	}

	terminator.closeCallback = self.tunneler.stateManager.AddEdgeSessionRemovedListener(response.Session.Token, func(token string) {
		if err := self.removeTerminator(terminator); err != nil {
			logger.WithError(err).Error("failed to remove terminator after edge session was removed")
		}
		go self.establishTerminatorWithRetry(terminator)
	})

	logger.WithField("terminatorId", response.TerminatorId).Info("created terminator")

	terminator.terminatorId = response.TerminatorId
	return nil
}

func (self *fabricProvider) removeTerminator(terminator *tunnelTerminator) error {
	msg := channel2.NewMessage(int32(edge_ctrl_pb.ContentType_RemoveTunnelTerminatorRequestType), []byte(terminator.terminatorId))
	responseMsg, err := self.factory.Channel().SendAndWaitWithTimeout(msg, self.factory.DefaultRequestTimeout())
	return xgress_common.CheckForFailureResult(responseMsg, err, edge_ctrl_pb.ContentType_RemoveTunnelTerminatorResponseType)
}

func (self *fabricProvider) updateTerminator(terminatorId string, cost *uint16, precedence *edge.Precedence) error {
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

	logger := logrus.WithField("terminator", terminatorId).
		WithField("precedence", request.Precedence).
		WithField("cost", request.Cost).
		WithField("updatingPrecedence", request.UpdatePrecedence).
		WithField("updatingCost", request.UpdateCost)

	logger.Debug("updating terminator")

	responseMsg, err := self.factory.Channel().SendForReply(request, self.factory.DefaultRequestTimeout())
	if err := xgress_common.CheckForFailureResult(responseMsg, err, edge_ctrl_pb.ContentType_UpdateTunnelTerminatorResponseType); err != nil {
		logger.WithError(err).Error("terminator update failed")
		return err
	}

	logger.Debug("terminator updated successfully")
	return nil
}

func (self *fabricProvider) sendHealthEvent(terminatorId string, checkPassed bool) error {
	msg := channel2.NewMessage(int32(edge_ctrl_pb.ContentType_TunnelHealthEventType), nil)
	msg.Headers[int32(edge_ctrl_pb.Header_TerminatorId)] = []byte(terminatorId)
	msg.PutBoolHeader(int32(edge_ctrl_pb.Header_CheckPassed), checkPassed)

	logger := logrus.WithField("terminator", terminatorId).
		WithField("checkPassed", checkPassed)
	logger.Debug("sending health event")

	if err := self.factory.Channel().Send(msg); err != nil {
		logger.WithError(err).Error("health event send failed")
	} else {
		logger.Debug("health event sent")
	}

	return nil
}

func (self *fabricProvider) requestServiceList(lastUpdateToken []byte) {
	msg := channel2.NewMessage(int32(edge_ctrl_pb.ContentType_ListServicesRequestType), lastUpdateToken)
	if err := self.factory.Channel().Send(msg); err != nil {
		logrus.WithError(err).Error("failed to send service list request to controller")
	}
}

type tunnelTerminator struct {
	provider      *fabricProvider
	context       tunnel.HostingContext
	address       string
	terminatorId  string
	closeCallback func()
	closed        concurrenz.AtomicBoolean
}

func (self *tunnelTerminator) SendHealthEvent(pass bool) error {
	return self.provider.sendHealthEvent(self.terminatorId, pass)
}

func (self *tunnelTerminator) Close() error {
	if self.closed.CompareAndSwap(false, true) {
		log := logrus.WithField("service", self.context.ServiceName()).
			WithField("routerId", self.provider.factory.id).
			WithField("terminator", self.terminatorId)

		log.Debug("closing tunnel terminator context")
		self.context.OnClose()

		log.Debug("unregistering session listener for tunnel terminator")
		self.closeCallback()

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
	return self.provider.updateTerminator(self.terminatorId, cost, precedence)
}
