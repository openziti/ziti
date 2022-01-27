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

package sync_strats

import (
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/lucsky/cuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/handler_edge_ctrl"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/build"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/util/debugz"
	"go.etcd.io/bbolt"
	"strings"
	"sync"
	"time"
)

const (
	RouterSyncStrategyInstant env.RouterSyncStrategyType = "instant"
)

var _ env.RouterSyncStrategy = &InstantStrategy{}

// InstantStrategyOptions is the options for the instant strategy.
// - MaxQueuedRouterConnects    - max number of router connected events to buffer
// - MaxQueuedClientHellos      - max number of client hello messages to buffer
// - RouterConnectWorkerCount   - max number of workers used to process router connections
// - SyncWorkerCount            - max number of workers used to send api sessions/session data
// - RouterTxBufferSize         - max number of messages buffered to be send to a router
// - HelloSendTimeout           - the max amount of time per worker to wait to send hellos
// - SessionChunkSize           - the number of sessions to send in each message
type InstantStrategyOptions struct {
	MaxQueuedRouterConnects  int32
	MaxQueuedClientHellos    int32
	RouterConnectWorkerCount int32
	SyncWorkerCount          int32
	RouterTxBufferSize       int
	HelloSendTimeout         time.Duration
	SessionChunkSize         int
}

// InstantStrategy assumes that on connect, the router requires and instant
// and full set of API Sessions. Send individual create, update, delete events for sessions after synchronization.
//
// This strategy uses a series of queues and workers to managed synchronization state. The order of events is as follows:
// 1. An edge router connects to the controller, triggering RouterConnected()
// 2. A RouterSender is created encapsulating the Edge Router, Router, and Sync State
// 3. The RouterSender is queued on the routerConnectedQueue channel which buffers up to options.MaxQueuedRouterConnects
// 4. The routerConnectedQueue is read and the edge server hello is sent
// 5. The controller waits for a client hello to be received via ReceiveClientHello message
// 6. The client hello is used to identity the RouterSender associated with the client and is queued on
//    the receivedClientHelloQueue channel which buffers up to options.MaxQueuedClientHellos
// 7. A startSynchronizeWorker will pick up the RouterSender from the receivedClientHelloQueue and being to
//    send data to the edge router via the RouterSender
type InstantStrategy struct {
	InstantStrategyOptions

	rtxMap *routerTxMap

	helloHandler  channel.TypedReceiveHandler
	resyncHandler channel.TypedReceiveHandler
	ae            *env.AppEnv

	routerConnectedQueue     chan *RouterSender
	receivedClientHelloQueue chan *RouterSender

	stop chan struct{}
}

func NewInstantStrategy(ae *env.AppEnv, options InstantStrategyOptions) *InstantStrategy {
	if options.MaxQueuedRouterConnects <= 0 {
		pfxlog.Logger().Panicf("MaxQueuedRouterConnects for InstantStrategy cannot be less than 1, got %d", options.MaxQueuedRouterConnects)
	}

	if options.MaxQueuedClientHellos <= 0 {
		pfxlog.Logger().Panicf("MaxQueuedClientHellos for InstantStrategy cannot be less than 1, got %d", options.MaxQueuedClientHellos)
	}

	if options.RouterConnectWorkerCount <= 0 {
		pfxlog.Logger().Panicf("RouterConnectWorkerCount for InstantStrategy cannot be less than 1, got %d", options.RouterConnectWorkerCount)
	}

	if options.SyncWorkerCount <= 0 {
		pfxlog.Logger().Panicf("SyncWorkerCount for InstantStrategy cannot be less than 1, got %d", options.SyncWorkerCount)
	}

	if options.RouterTxBufferSize < 0 {
		pfxlog.Logger().Panicf("RouterTxBufferSize for InstantStrategy cannot be less than 0, got %d", options.MaxQueuedRouterConnects)
	}

	if options.SessionChunkSize <= 0 {
		pfxlog.Logger().Panicf("SessionChunkSize for InstantStrategy cannot be less than 1, got %d", options.SessionChunkSize)
	}

	strategy := &InstantStrategy{
		InstantStrategyOptions: options,
		rtxMap: &routerTxMap{
			internalMap: &sync.Map{},
		},
		ae:                       ae,
		routerConnectedQueue:     make(chan *RouterSender, options.MaxQueuedRouterConnects),
		receivedClientHelloQueue: make(chan *RouterSender, options.MaxQueuedClientHellos),

		stop: make(chan struct{}, 0),
	}

	strategy.helloHandler = handler_edge_ctrl.NewHelloHandler(ae, strategy.ReceiveClientHello)
	strategy.resyncHandler = handler_edge_ctrl.NewResyncHandler(ae, strategy.ReceiveResync)

	for i := int32(0); i < options.RouterConnectWorkerCount; i++ {
		go strategy.startHandleRouterConnectWorker()
	}

	for i := int32(0); i < options.SyncWorkerCount; i++ {
		go strategy.startSynchronizeWorker()
	}

	return strategy
}

func (strategy *InstantStrategy) GetEdgeRouterState(id string) env.RouterStateValues {
	return strategy.rtxMap.GetState(id)
}

func (strategy *InstantStrategy) Type() env.RouterSyncStrategyType {
	return RouterSyncStrategyInstant
}

func (strategy *InstantStrategy) Stop() {
	if strategy.stop != nil {
		close(strategy.stop)
		strategy.stop = nil
	}
}

func (strategy *InstantStrategy) RouterConnected(edgeRouter *model.EdgeRouter, router *network.Router) {
	rtx := newRouterSender(edgeRouter, router, strategy.RouterTxBufferSize)
	rtx.SetSyncStatus(env.RouterSyncQueued)
	rtx.SetIsOnline(true)

	log := pfxlog.Logger().WithField("sync_strategy", strategy.Type()).
		WithField("syncStatus", rtx.SyncStatus()).
		WithField("routerId", rtx.Router.Id).
		WithField("routerName", rtx.Router.Name).
		WithField("routerFingerprint", rtx.Router.Fingerprint)

	log.Info("edge router connected, adding to sync routerConnectedQueue")

	strategy.rtxMap.Add(router.Id, rtx)

	strategy.routerConnectedQueue <- rtx
}

func (strategy *InstantStrategy) RouterDisconnected(router *network.Router) {
	pfxlog.Logger().WithField("routerId", router.Id).
		WithField("routerName", router.Name).
		WithField("routerFingerprint", router.Fingerprint).
		Infof("edge router [%s] disconnecting", router.Id)
	strategy.rtxMap.Remove(router.Id)
}

func (strategy *InstantStrategy) GetReceiveHandlers() []channel.TypedReceiveHandler {
	var result []channel.TypedReceiveHandler
	if strategy.helloHandler != nil {
		result = append(result, strategy.helloHandler)
	}
	if strategy.resyncHandler != nil {
		result = append(result, strategy.resyncHandler)
	}
	return result
}

func (strategy *InstantStrategy) ApiSessionAdded(apiSession *persistence.ApiSession) {
	logger := pfxlog.Logger().WithField("strategy", strategy.Type())

	apiSessionProto, err := apiSessionToProto(strategy.ae, apiSession.Token, apiSession.IdentityId, apiSession.Id)

	if err != nil {
		logger.WithField("apiSessionId", apiSession.Id).
			Errorf("error for individual api session added, could not convert to proto: %v", err)
		return
	}

	state := &InstantSyncState{
		Id:       cuid.New(),
		IsLast:   true,
		Sequence: 0,
	}

	strategy.rtxMap.Range(func(rtx *RouterSender) {
		_ = strategy.sendApiSessionAdded(rtx, false, state, []*edge_ctrl_pb.ApiSession{apiSessionProto})
	})
}

func (strategy *InstantStrategy) ApiSessionUpdated(apiSession *persistence.ApiSession, _ *persistence.ApiSessionCertificate) {
	logger := pfxlog.Logger().WithField("strategy", strategy.Type())

	apiSessionProto, err := apiSessionToProto(strategy.ae, apiSession.Token, apiSession.IdentityId, apiSession.Id)

	if err != nil {
		logger.WithField("apiSessionId", apiSession.Id).
			Errorf("error for individual api session added, could not convert to proto: %v", err)
		return
	}

	apiSessionAdded := &edge_ctrl_pb.ApiSessionAdded{
		IsFullState: false,
		ApiSessions: []*edge_ctrl_pb.ApiSession{apiSessionProto},
	}

	strategy.rtxMap.Range(func(rtx *RouterSender) {
		content, _ := proto.Marshal(apiSessionAdded)
		msg := channel.NewMessage(env.ApiSessionUpdatedType, content)
		msg.Headers[env.SyncStrategyTypeHeader] = []byte(strategy.Type())
		msg.Headers[env.SyncStrategyStateHeader] = nil
		_ = rtx.Send(msg)
	})
}

func (strategy *InstantStrategy) ApiSessionDeleted(apiSession *persistence.ApiSession) {
	sessionRemoved := &edge_ctrl_pb.ApiSessionRemoved{
		Tokens: []string{apiSession.Token},
	}

	strategy.rtxMap.Range(func(rtx *RouterSender) {
		content, _ := proto.Marshal(sessionRemoved)
		msg := channel.NewMessage(env.ApiSessionRemovedType, content)
		_ = rtx.Send(msg)
	})
}

func (strategy *InstantStrategy) SessionDeleted(session *persistence.Session) {
	sessionRemoved := &edge_ctrl_pb.SessionRemoved{
		Tokens: []string{session.Token},
	}

	strategy.rtxMap.Range(func(rtx *RouterSender) {
		content, _ := proto.Marshal(sessionRemoved)
		msg := channel.NewMessage(env.SessionRemovedType, content)
		_ = rtx.Send(msg)
	})
}

func (strategy *InstantStrategy) startHandleRouterConnectWorker() {
	for {
		select {
		case <-strategy.stop:
			return
		case rtx := <-strategy.routerConnectedQueue:
			func() {
				defer func() {
					if r := recover(); r != nil {
						pfxlog.Logger().Errorf("router connect worker panic, worker recovering: %v\n%v", r, debugz.GenerateLocalStack())
						rtx.SetSyncStatus(env.RouterSyncError)
						rtx.logger().Errorf("panic during edge router connection, sync failed")
					}
				}()
				strategy.hello(rtx)
			}()
		}
	}
}

func (strategy *InstantStrategy) startSynchronizeWorker() {
	for {
		select {
		case <-strategy.stop:
			return
		case rtx := <-strategy.receivedClientHelloQueue:
			func() {
				defer func() {
					if r := recover(); r != nil {
						pfxlog.Logger().Errorf("sync worker panic, worker recovering: %v\n%v", r, debugz.GenerateLocalStack())
						rtx.SetSyncStatus(env.RouterSyncError)
						rtx.logger().Errorf("panic during edge router sync, sync failed")
					}
				}()
				strategy.synchronize(rtx)
			}()
		}
	}
}

func (strategy *InstantStrategy) hello(rtx *RouterSender) {
	logger := rtx.logger().WithField("strategy", strategy.Type())

	logger.Info("edge router sync starting")

	if rtx.Router.Control.IsClosed() {
		rtx.SetSyncStatus(env.RouterSyncDisconnected)
		logger.WithField("syncStatus", rtx.SyncStatus()).Info("edge router sync aborting, edge router disconnected before sync started")
		return
	}

	rtx.SetSyncStatus(env.RouterSyncHello)
	logger.WithField("syncStatus", rtx.SyncStatus()).Info("sending edge router hello")
	strategy.sendHello(rtx)
}

func (strategy *InstantStrategy) sendHello(rtx *RouterSender) {
	logger := rtx.logger().WithField("strategy", strategy.Type())
	serverVersion := build.GetBuildInfo().Version()
	serverHello := &edge_ctrl_pb.ServerHello{
		Version: serverVersion,
	}

	buf, err := proto.Marshal(serverHello)
	if err != nil {
		logger.WithError(err).Error("could not marshal serverHello")
		return
	}

	if err = channel.NewMessage(env.ServerHelloType, buf).WithTimeout(strategy.HelloSendTimeout).Send(rtx.Router.Control); err != nil {
		if rtx.Router.Control.IsClosed() {
			rtx.SetSyncStatus(env.RouterSyncDisconnected)
			rtx.logger().WithError(err).Error("timed out sending serverHello message for edge router, connection closed, giving up")
		} else {
			rtx.SetSyncStatus(env.RouterSyncHelloTimeout)
			rtx.logger().WithError(err).Error("timed out sending serverHello message for edge router, queuing again")
			go func() {
				strategy.routerConnectedQueue <- rtx
			}()
		}
	}
}

func (strategy *InstantStrategy) ReceiveResync(r *network.Router, _ *edge_ctrl_pb.RequestClientReSync) {
	rtx := strategy.rtxMap.Get(r.Id)

	if rtx == nil {
		pfxlog.Logger().
			WithField("strategy", strategy.Type()).
			WithField("routerId", r.Id).
			WithField("routerName", r.Name).
			Error("received resync from router that is currently not tracked by the strategy, dropping resync")
		return
	}

	rtx.SetSyncStatus(env.RouterSyncResyncWait)

	rtx.logger().WithField("strategy", strategy.Type()).Info("received resync from router, queuing")

	strategy.receivedClientHelloQueue <- rtx
}

func (strategy *InstantStrategy) ReceiveClientHello(r *network.Router, respHello *edge_ctrl_pb.ClientHello) {
	rtx := strategy.rtxMap.Get(r.Id)

	if rtx == nil {
		pfxlog.Logger().
			WithField("strategy", strategy.Type()).
			WithField("routerId", r.Id).
			WithField("routerName", r.Name).
			Error("received hello from router that is currently not tracked by the strategy, dropping hello")
		return
	}
	rtx.SetSyncStatus(env.RouterSyncHelloWait)

	logger := rtx.logger().WithField("strategy", strategy.Type()).
		WithField("hostname", respHello.Hostname).
		WithField("protocols", respHello.Protocols).
		WithField("protocolPorts", respHello.ProtocolPorts).
		WithField("data", respHello.Data)

	serverVersion := build.GetBuildInfo().Version()

	if r.VersionInfo != nil {
		logger = logger.WithField("version", r.VersionInfo.Version).
			WithField("revision", r.VersionInfo.Revision).
			WithField("buildDate", r.VersionInfo.BuildDate).
			WithField("os", r.VersionInfo.OS).
			WithField("arch", r.VersionInfo.Arch)
	}

	protocols := map[string]string{}
	for _, p := range respHello.ProtocolPorts {
		parts := strings.Split(p, ":")
		ingressUrl := fmt.Sprintf("%s://%s:%s", parts[0], respHello.Hostname, parts[1])
		protocols[parts[0]] = ingressUrl
	}

	rtx.SetHostname(respHello.Hostname)
	rtx.SetProtocols(protocols)
	rtx.SetVersionInfo(*r.VersionInfo)

	logger.Infof("edge router sent hello with version [%s] to controller with version [%s]", respHello.Version, serverVersion)
	strategy.receivedClientHelloQueue <- rtx
}

func (strategy *InstantStrategy) synchronize(rtx *RouterSender) {
	defer func() {
		rtx.logger().WithField("strategy", strategy.Type()).Infof("exiting synchronization, final status: %s", rtx.SyncStatus())
	}()

	if rtx.Router.Control.IsClosed() {
		rtx.SetSyncStatus(env.RouterSyncDisconnected)
		rtx.logger().WithField("strategy", strategy.Type()).Error("attempting to start synchronization with edge router, but it has disconnected")
	}

	rtx.SetSyncStatus(env.RouterSynInProgress)
	logger := rtx.logger().WithField("strategy", strategy.Type())
	logger.Info("started synchronizing edge router")

	chunkSize := 100
	err := strategy.ae.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		var apiSessions []*edge_ctrl_pb.ApiSession

		state := &InstantSyncState{
			Id:       cuid.New(),
			IsLast:   true,
			Sequence: 0,
		}

		for cursor := strategy.ae.GetStores().ApiSession.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
			current := cursor.Current()

			apiSession, err := strategy.ae.GetStores().ApiSession.LoadOneById(tx, string(current))

			if err != nil {
				logger.WithError(err).WithField("apiSessionId", string(current)).Errorf("error querying api session [%s]: %v", string(current), err)
				continue
			}

			apiSessionProto, err := apiSessionToProtoWithTx(tx, strategy.ae, apiSession.Token, apiSession.IdentityId, apiSession.Id)

			if err != nil {
				logger.WithError(err).WithField("apiSessionId", string(current)).Errorf("error turning apiSession [%s] into proto: %v", string(current), err)
				continue
			}

			apiSessions = append(apiSessions, apiSessionProto)

			if len(apiSessions) >= chunkSize {
				state.IsLast = !cursor.IsValid()
				if err = strategy.sendApiSessionAdded(rtx, true, state, apiSessions); err != nil {
					return err
				}

				state.Sequence = state.Sequence + 1
				apiSessions = []*edge_ctrl_pb.ApiSession{}
			}
		}

		if len(apiSessions) > 0 {
			state.IsLast = true
			if err := strategy.sendApiSessionAdded(rtx, true, state, apiSessions); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		logger.WithError(err).Error("failure synchronizing api sessions")
		rtx.SetSyncStatus(env.RouterSyncError)
	} else {
		rtx.SetSyncStatus(env.RouterSyncDone)
	}
}

func (strategy *InstantStrategy) sendApiSessionAdded(rtx *RouterSender, isFullState bool, state *InstantSyncState, apiSessions []*edge_ctrl_pb.ApiSession) error {
	stateBytes, _ := json.Marshal(state)

	msgContent := &edge_ctrl_pb.ApiSessionAdded{
		IsFullState: isFullState,
		ApiSessions: apiSessions,
	}

	msgContentBytes, _ := proto.Marshal(msgContent)

	msg := channel.NewMessage(env.ApiSessionAddedType, msgContentBytes)

	msg.Headers[env.SyncStrategyTypeHeader] = []byte(strategy.Type())
	msg.Headers[env.SyncStrategyStateHeader] = stateBytes

	return rtx.Send(msg)
}

type InstantSyncState struct {
	Id       string `json:"id"`       //unique id for the sync attempt
	IsLast   bool   `json:"isLast"`   //
	Sequence int    `json:"sequence"` //increasing id from 0 per id for the
}
