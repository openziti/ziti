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

package sync_strats

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/lucsky/cuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/foundation/v2/genext"
	nfPem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/build"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/handler_edge_ctrl"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/controller/persistence"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	RouterSyncStrategyInstant env.RouterSyncStrategyType = "instant"
	ZdbIndexKey                                          = "index"
	ZdbKey                                               = "zdb"
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
//  1. An edge router connects to the controller, triggering RouterConnected()
//  2. A RouterSender is created encapsulating the Edge Router, Router, and Sync State
//  3. The RouterSender is queued on the routerConnectedQueue channel which buffers up to options.MaxQueuedRouterConnects
//  4. The routerConnectedQueue is read and the edge server hello is sent
//  5. The controller waits for a client hello to be received via ReceiveClientHello message
//  6. The client hello is used to identity the RouterSender associated with the client and is queued on
//     the receivedClientHelloQueue channel which buffers up to options.MaxQueuedClientHellos
//  7. A startSynchronizeWorker will pick up the RouterSender from the receivedClientHelloQueue and being to
//     send data to the edge router via the RouterSender
type InstantStrategy struct {
	InstantStrategyOptions

	rtxMap *routerTxMap

	helloHandler  channel.TypedReceiveHandler
	resyncHandler channel.TypedReceiveHandler
	ae            *env.AppEnv

	routerConnectedQueue     chan *RouterSender
	receivedClientHelloQueue chan *RouterSender

	indexProvider IndexProvider

	stopNotify chan struct{}
	stopped    atomic.Bool
	*common.RouterDataModel
	servicePolicyHandler *constraintToIndexedEvents[*persistence.ServicePolicy]
	identityHandler      *constraintToIndexedEvents[*persistence.Identity]
	postureCheckHandler  *constraintToIndexedEvents[*persistence.PostureCheck]
	serviceHandler       *constraintToIndexedEvents[*persistence.EdgeService]
	caHandler            *constraintToIndexedEvents[*persistence.Ca]
	revocationHandler    *constraintToIndexedEvents[*persistence.Revocation]
}

// Initialize implements RouterDataModelCache
func (strategy *InstantStrategy) Initialize(logSize uint64, bufferSize uint) error {
	strategy.RouterDataModel = common.NewSenderRouterDataModel(logSize, bufferSize)

	if strategy.ae.HostController.IsRaftEnabled() {
		strategy.indexProvider = &RaftIndexProvider{}
	} else {
		strategy.indexProvider = &NonHaIndexProvider{
			ae: strategy.ae,
		}
	}

	err := strategy.BuildAll()

	if err != nil {
		return err
	}

	go strategy.handleRouterModelEvents(strategy.RouterDataModel.NewListener())

	//policy create/delete/update
	strategy.servicePolicyHandler = &constraintToIndexedEvents[*persistence.ServicePolicy]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.ServicePolicyCreate,
		updateHandler: strategy.ServicePolicyUpdate,
		deleteHandler: strategy.ServicePolicyDelete,
	}
	strategy.ae.GetStores().ServicePolicy.AddEntityConstraint(strategy.servicePolicyHandler)

	//identity create/delete/update
	strategy.identityHandler = &constraintToIndexedEvents[*persistence.Identity]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.IdentityCreate,
		updateHandler: strategy.IdentityUpdate,
		deleteHandler: strategy.IdentityDelete,
	}
	strategy.ae.GetStores().Identity.AddEntityConstraint(strategy.identityHandler)

	//posture check create/delete/update
	strategy.postureCheckHandler = &constraintToIndexedEvents[*persistence.PostureCheck]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.PostureCheckCreate,
		updateHandler: strategy.PostureCheckUpdate,
		deleteHandler: strategy.PostureCheckDelete,
	}
	strategy.ae.GetStores().PostureCheck.AddEntityConstraint(strategy.postureCheckHandler)

	//service create/delete/update
	strategy.serviceHandler = &constraintToIndexedEvents[*persistence.EdgeService]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.ServiceCreate,
		updateHandler: strategy.ServiceUpdate,
		deleteHandler: strategy.ServiceDelete,
	}
	strategy.ae.GetStores().EdgeService.AddEntityConstraint(strategy.serviceHandler)

	//ca create/delete/update
	strategy.caHandler = &constraintToIndexedEvents[*persistence.Ca]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.CaCreate,
		updateHandler: strategy.CaUpdate,
		deleteHandler: strategy.CaDelete,
	}
	strategy.ae.GetStores().Ca.AddEntityConstraint(strategy.caHandler)

	//ca create/delete/update
	strategy.revocationHandler = &constraintToIndexedEvents[*persistence.Revocation]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.RevocationCreate,
		updateHandler: strategy.RevocationUpdate,
		deleteHandler: strategy.RevocationDelete,
	}
	strategy.ae.GetStores().Ca.AddEntityConstraint(strategy.caHandler)

	return nil
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
			internalMap: cmap.New[*RouterSender](),
		},
		ae:                       ae,
		routerConnectedQueue:     make(chan *RouterSender, options.MaxQueuedRouterConnects),
		receivedClientHelloQueue: make(chan *RouterSender, options.MaxQueuedClientHellos),
		RouterDataModel:          common.NewSenderRouterDataModel(10000, 10000),
		stopNotify:               make(chan struct{}),
	}

	err := strategy.Initialize(10000, 1000)

	if err != nil {
		pfxlog.Logger().WithError(err).Fatal("could not build initial data model for router synchronization")
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
	if strategy.stopped.CompareAndSwap(false, true) {
		close(strategy.stopNotify)
	}
}

func (strategy *InstantStrategy) RouterConnected(edgeRouter *model.EdgeRouter, router *network.Router) {
	log := pfxlog.Logger().WithField("sync_strategy", strategy.Type()).
		WithField("routerId", router.Id).
		WithField("routerName", router.Name).
		WithField("routerFingerprint", *router.Fingerprint)

	//connecting router has closed control channel
	if router.Control.IsClosed() {
		log.Errorf("connecting router has closed control channel [id: %s], ignoring", router.Id)
		return
	}

	existingRtx := strategy.rtxMap.Get(router.Id)

	//same channel, do nothing
	if existingRtx != nil && existingRtx.Router.Control == router.Control {
		log.Errorf("duplicate router connection detected [id: %s], channels are the same, ignoring", router.Id)
		return
	}

	rtx := newRouterSender(edgeRouter, router, strategy.RouterTxBufferSize)
	rtx.SetSyncStatus(env.RouterSyncQueued)
	rtx.SetIsOnline(true)

	log.WithField("syncStatus", rtx.SyncStatus()).Info("edge router connected, adding to sync routerConnectedQueue")

	strategy.rtxMap.Add(router.Id, rtx)

	strategy.routerConnectedQueue <- rtx
}

func (strategy *InstantStrategy) RouterDisconnected(router *network.Router) {
	log := pfxlog.Logger().WithField("sync_strategy", strategy.Type()).
		WithField("routerId", router.Id).
		WithField("routerName", router.Name).
		WithField("routerFingerprint", genext.OrDefault(router.Fingerprint))

	existingRtx := strategy.rtxMap.Get(router.Id)

	if existingRtx == nil {
		log.Infof("edge router [%s] disconnect event, but no rtx found, ignoring", router.Id)
		return
	}

	if existingRtx.Router.Control != router.Control && !existingRtx.Router.Control.IsClosed() {
		log.Infof("edge router [%s] disconnect event, but channels do not match and existing channel is still open, ignoring", router.Id)
		return
	}

	log.Infof("edge router [%s] disconnect event, router rtx removed", router.Id)
	existingRtx.SetIsOnline(false)
	strategy.rtxMap.Remove(router)
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

	logger.WithField("apiSessionId", apiSession.Id).WithField("fingerprints", apiSessionProto.CertFingerprints).Debug("adding apiSession")

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

	logger.WithField("apiSessionId", apiSession.Id).WithField("fingerprints", apiSessionProto.CertFingerprints).Debug("updating apiSession")

	apiSessionAdded := &edge_ctrl_pb.ApiSessionUpdated{
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
		case <-strategy.stopNotify:
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
		case <-strategy.stopNotify:
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
		Data: map[string]string{
			"instant":     "true",
			"routerModel": "true",
		},
		ByteData: map[string][]byte{},
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

	rtx.RouterModelIndex = nil

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
		WithField("protocols", respHello.Protocols).
		WithField("protocolPorts", respHello.ProtocolPorts).
		WithField("listeners", respHello.Listeners).
		WithField("data", respHello.Data)

	if respHello.Data != nil && respHello.Data[common.DataRouterModel] == "true" {
		rtx.SupportsRouterModel = true

		indexStr := respHello.Data[common.DataRouterModelIndex]

		if len(indexStr) > 0 {
			index, err := strconv.ParseUint(indexStr, 10, 64)

			if err == nil {
				rtx.RouterModelIndex = &index
			}
		}
	}

	serverVersion := build.GetBuildInfo().Version()

	if r.VersionInfo != nil {
		logger = logger.WithField("version", r.VersionInfo.Version).
			WithField("revision", r.VersionInfo.Revision).
			WithField("buildDate", r.VersionInfo.BuildDate).
			WithField("os", r.VersionInfo.OS).
			WithField("arch", r.VersionInfo.Arch)
	}

	protocols := map[string]string{}

	if len(respHello.Listeners) > 0 {
		for _, listener := range respHello.Listeners {
			protocols[listener.Advertise.Protocol] = fmt.Sprintf("%s://%s:%d", listener.Advertise.Protocol, listener.Advertise.Hostname, listener.Advertise.Port)
		}
	} else {
		for idx, protocol := range respHello.Protocols {
			if len(respHello.ProtocolPorts) > idx {
				port := respHello.ProtocolPorts[idx]
				ingressUrl := fmt.Sprintf("%s://%s:%s", protocol, respHello.Hostname, port)
				protocols[protocol] = ingressUrl
			}
		}
	}

	rtx.SetHostname(respHello.Hostname)
	rtx.SetProtocols(protocols)
	rtx.SetVersionInfo(*r.VersionInfo)

	logger.Infof("edge router sent hello with version [%s] to controller with version [%s]", respHello.Version, serverVersion)
	strategy.receivedClientHelloQueue <- rtx
}

func (strategy *InstantStrategy) synchronize(rtx *RouterSender) {
	defer func() {
		rtx.logger().WithField("strategy", strategy.Type()).WithField("SupportsRouterModel", rtx.SupportsRouterModel).Infof("exiting synchronization, final status: %s", rtx.SyncStatus())
	}()

	if rtx.Router.Control.IsClosed() {
		rtx.SetSyncStatus(env.RouterSyncDisconnected)
		rtx.logger().WithField("strategy", strategy.Type()).WithField("SupportsRouterModel", rtx.SupportsRouterModel).Error("attempting to start synchronization with edge router, but it has disconnected")
	}

	rtx.SetSyncStatus(env.RouterSynInProgress)
	logger := rtx.logger().WithField("strategy", strategy.Type()).WithField("SupportsRouterModel", rtx.SupportsRouterModel)
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
		return
	}

	if rtx.SupportsRouterModel {
		replayFrom := rtx.RouterModelIndex

		if replayFrom != nil {
			rtx.RouterModelIndex = nil
			events, ok := strategy.RouterDataModel.ReplayFrom(*replayFrom)

			if ok {
				var err error
				for _, curEvent := range events {
					err = strategy.sendDataStatEvent(rtx, curEvent)
					if err != nil {
						break
					}
				}
			}

			// no error sync is done, if err try full state
			if err == nil {
				rtx.SetSyncStatus(env.RouterSyncDone)
				return
			}

			pfxlog.Logger().WithError(err).Error("could not send events for router sync, attempting full state")
		}

		//full sync
		dataState := strategy.RouterDataModel.GetDataState()

		if dataState == nil {
			return
		}
		dataState.EndIndex = strategy.indexProvider.CurrentIndex()

		if err := strategy.sendDataState(rtx, dataState); err != nil {
			logger.WithError(err).Error("failure sending full data state")
			rtx.SetSyncStatus(env.RouterSyncError)
			return
		}
	}

	rtx.SetSyncStatus(env.RouterSyncDone)
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

func (strategy *InstantStrategy) handleRouterModelEvents(eventChannel <-chan *edge_ctrl_pb.DataState_Event) {
	for {
		select {
		case newEvent := <-eventChannel:
			strategy.rtxMap.Range(func(rtx *RouterSender) {

				if !rtx.SupportsRouterModel {
					return
				}

				err := strategy.sendDataStatEvent(rtx, newEvent)

				if err != nil {
					pfxlog.Logger().WithError(err).WithField("routerId", rtx.Router.Id).Error("error sending data state to router")
				}
			})
		case <-strategy.ae.HostController.GetCloseNotifyChannel():
			return
		}
	}
}

type InstantSyncState struct {
	Id       string `json:"id"`       //unique id for the sync attempt
	IsLast   bool   `json:"isLast"`   //
	Sequence int    `json:"sequence"` //increasing id from 0 per id for the
}

func (strategy *InstantStrategy) BuildServicePolicies(tx *bbolt.Tx) error {
	for cursor := strategy.ae.GetStores().ServicePolicy.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		currentBytes := cursor.Current()
		currentId := string(currentBytes)

		storeModel, err := strategy.ae.GetStores().ServicePolicy.LoadOneById(tx, currentId)

		if err != nil {
			return err
		}

		servicePolicy := newServicePolicy(tx, strategy.ae, storeModel)

		newModel := &edge_ctrl_pb.DataState_Event_ServicePolicy{ServicePolicy: servicePolicy}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}
		strategy.ApplyServicePolicyEvent(newEvent, newModel)
	}

	return nil
}

func (strategy *InstantStrategy) BuildPublicKeys(tx *bbolt.Tx) error {
	for _, x509Cert := range strategy.ae.HostController.GetPeerSigners() {
		publicKey := newPublicKey(x509Cert.Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_JWTValidation})
		newModel := &edge_ctrl_pb.DataState_Event_PublicKey{PublicKey: publicKey}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}

		strategy.ApplyPublicKeyEvent(newEvent, newModel)
	}

	caPEMs := strategy.ae.Config.CaPems()
	caCerts := nfPem.PemBytesToCertificates(caPEMs)

	for _, caCert := range caCerts {
		publicKey := newPublicKey(caCert.Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation})
		newModel := &edge_ctrl_pb.DataState_Event_PublicKey{PublicKey: publicKey}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}

		strategy.ApplyPublicKeyEvent(newEvent, newModel)
	}

	for cursor := strategy.ae.GetStores().Ca.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		currentBytes := cursor.Current()
		currentId := string(currentBytes)

		ca, err := strategy.ae.GetStores().Ca.LoadOneById(tx, currentId)

		if err != nil {
			return err
		}

		certs := nfPem.PemStringToCertificates(ca.CertPem)

		if len(certs) == 0 {
			continue
		}

		publicKey := newPublicKey(certs[0].Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation})

		newModel := &edge_ctrl_pb.DataState_Event_PublicKey{PublicKey: publicKey}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}
		strategy.ApplyPublicKeyEvent(newEvent, newModel)
	}

	return nil
}

func (strategy *InstantStrategy) BuildAll() error {
	err := strategy.ae.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		if err := strategy.BuildIdentities(tx); err != nil {
			return err
		}

		if err := strategy.BuildServices(tx); err != nil {
			return err
		}

		if err := strategy.BuildPostureChecks(tx); err != nil {
			return err
		}

		if err := strategy.BuildServicePolicies(tx); err != nil {
			return err
		}

		if err := strategy.BuildPublicKeys(tx); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (strategy *InstantStrategy) BuildIdentities(tx *bbolt.Tx) error {
	for cursor := strategy.ae.GetStores().Identity.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		currentBytes := cursor.Current()
		currentId := string(currentBytes)

		identity, err := newIdentityById(tx, strategy.ae, currentId)

		if err != nil {
			return err

		}

		newModel := &edge_ctrl_pb.DataState_Event_Identity{Identity: identity}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}
		strategy.ApplyIdentityEvent(newEvent, newModel)
	}

	return nil
}

func (strategy *InstantStrategy) BuildServices(tx *bbolt.Tx) error {
	for cursor := strategy.ae.GetStores().EdgeService.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		currentBytes := cursor.Current()
		currentId := string(currentBytes)

		service, err := newServiceById(tx, strategy.ae, currentId)

		if err != nil {
			return err
		}

		newModel := &edge_ctrl_pb.DataState_Event_Service{Service: service}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}
		strategy.ApplyServiceEvent(newEvent, newModel)
	}

	return nil
}

func (strategy *InstantStrategy) BuildPostureChecks(tx *bbolt.Tx) error {
	for cursor := strategy.ae.GetStores().PostureCheck.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		currentBytes := cursor.Current()
		currentId := string(currentBytes)

		postureCheck, err := newPostureCheckById(tx, strategy.ae, currentId)

		if err != nil {
			return err
		}

		newModel := &edge_ctrl_pb.DataState_Event_PostureCheck{PostureCheck: postureCheck}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}
		strategy.ApplyPostureCheckEvent(newEvent, newModel)
	}
	return nil
}

func newIdentityById(tx *bbolt.Tx, ae *env.AppEnv, id string) (*edge_ctrl_pb.DataState_Identity, error) {
	identityModel, err := ae.GetStores().Identity.LoadOneById(tx, id)

	if err != nil {
		return nil, err
	}

	return newIdentity(identityModel), nil
}

func newIdentity(identityModel *persistence.Identity) *edge_ctrl_pb.DataState_Identity {
	return &edge_ctrl_pb.DataState_Identity{
		Id:   identityModel.Id,
		Name: identityModel.Name,
	}
}

func newServicePolicy(tx *bbolt.Tx, env *env.AppEnv, storeModel *persistence.ServicePolicy) *edge_ctrl_pb.DataState_ServicePolicy {
	servicePolicy := &edge_ctrl_pb.DataState_ServicePolicy{
		Id:   storeModel.Id,
		Name: storeModel.Name,
	}

	result := env.GetManagers().ServicePolicy.ListAssociatedIds(tx, storeModel.Id)

	servicePolicy.PostureCheckIds = result.PostureCheckIds
	servicePolicy.ServiceIds = result.ServiceIds
	servicePolicy.IdentityIds = result.IdentityIds

	return servicePolicy
}

func newServiceById(tx *bbolt.Tx, ae *env.AppEnv, id string) (*edge_ctrl_pb.DataState_Service, error) {
	storeModel, err := ae.GetStores().EdgeService.LoadOneById(tx, id)

	if err != nil {
		return nil, err
	}

	return newService(storeModel), nil
}

func newService(storeModel *persistence.EdgeService) *edge_ctrl_pb.DataState_Service {
	return &edge_ctrl_pb.DataState_Service{
		Id:   storeModel.Id,
		Name: storeModel.Name,
	}
}

func newPublicKey(data []byte, format edge_ctrl_pb.DataState_PublicKey_Format, usages []edge_ctrl_pb.DataState_PublicKey_Usage) *edge_ctrl_pb.DataState_PublicKey {
	return &edge_ctrl_pb.DataState_PublicKey{
		Data:   data,
		Kid:    fmt.Sprintf("%x", sha1.Sum(data)),
		Usages: usages,
		Format: format,
	}
}

func newPostureCheckById(tx *bbolt.Tx, ae *env.AppEnv, id string) (*edge_ctrl_pb.DataState_PostureCheck, error) {
	postureModel, err := ae.GetStores().PostureCheck.LoadOneById(tx, id)

	if err != nil {
		return nil, err
	}
	return newPostureCheck(postureModel), nil
}

func newPostureCheck(postureModel *persistence.PostureCheck) *edge_ctrl_pb.DataState_PostureCheck {
	newVal := &edge_ctrl_pb.DataState_PostureCheck{
		Id:     postureModel.Id,
		Name:   postureModel.Name,
		TypeId: postureModel.TypeId,
	}

	switch subType := postureModel.SubType.(type) {
	case *persistence.PostureCheckProcess:
		newVal.Subtype = &edge_ctrl_pb.DataState_PostureCheck_Process_{
			Process: &edge_ctrl_pb.DataState_PostureCheck_Process{
				OsType:       subType.OperatingSystem,
				Path:         subType.Path,
				Hashes:       subType.Hashes,
				Fingerprints: []string{subType.Fingerprint},
			},
		}
	case *persistence.PostureCheckProcessMulti:
		processList := &edge_ctrl_pb.DataState_PostureCheck_ProcessMulti_{
			ProcessMulti: &edge_ctrl_pb.DataState_PostureCheck_ProcessMulti{
				Semantic: subType.Semantic,
			},
		}

		for _, process := range subType.Processes {
			newProc := &edge_ctrl_pb.DataState_PostureCheck_Process{
				OsType:       process.OsType,
				Path:         process.Path,
				Hashes:       process.Hashes,
				Fingerprints: process.SignerFingerprints,
			}

			processList.ProcessMulti.Processes = append(processList.ProcessMulti.Processes, newProc)
		}

		newVal.Subtype = processList
	case *persistence.PostureCheckMfa:
		newVal.Subtype = &edge_ctrl_pb.DataState_PostureCheck_Mfa_{
			Mfa: &edge_ctrl_pb.DataState_PostureCheck_Mfa{
				TimeoutSeconds:        subType.TimeoutSeconds,
				PromptOnWake:          subType.PromptOnWake,
				PromptOnUnlock:        subType.PromptOnUnlock,
				IgnoreLegacyEndpoints: subType.IgnoreLegacyEndpoints,
			},
		}

	case *persistence.PostureCheckWindowsDomains:
		newVal.Subtype = &edge_ctrl_pb.DataState_PostureCheck_Domains_{
			Domains: &edge_ctrl_pb.DataState_PostureCheck_Domains{
				Domains: subType.Domains,
			},
		}
	case *persistence.PostureCheckMacAddresses:
		newVal.Subtype = &edge_ctrl_pb.DataState_PostureCheck_Mac_{
			Mac: &edge_ctrl_pb.DataState_PostureCheck_Mac{
				MacAddresses: subType.MacAddresses,
			},
		}
	case *persistence.PostureCheckOperatingSystem:

		osList := &edge_ctrl_pb.DataState_PostureCheck_OsList{}

		for _, os := range subType.OperatingSystems {
			newOs := &edge_ctrl_pb.DataState_PostureCheck_Os{
				OsType:     os.OsType,
				OsVersions: os.OsVersions,
			}

			osList.OsList = append(osList.OsList, newOs)
		}

		newVal.Subtype = &edge_ctrl_pb.DataState_PostureCheck_OsList_{
			OsList: osList,
		}
	}

	return newVal
}

func actionToName(action edge_ctrl_pb.DataState_Action) string {
	switch action {
	case edge_ctrl_pb.DataState_Create:
		return "CREATE"
	case edge_ctrl_pb.DataState_Update:
		return "UPDATE"
	case edge_ctrl_pb.DataState_Delete:
		return "DELETE"
	}

	return "UNKNOWN"
}

func (strategy *InstantStrategy) ServicePolicyCreate(index uint64, servicePolicy *persistence.ServicePolicy) {
	strategy.handleServicePolicy(index, edge_ctrl_pb.DataState_Create, servicePolicy)
}

func (strategy *InstantStrategy) ServicePolicyUpdate(index uint64, servicePolicy *persistence.ServicePolicy) {
	strategy.handleServicePolicy(index, edge_ctrl_pb.DataState_Update, servicePolicy)
}

func (strategy *InstantStrategy) ServicePolicyDelete(index uint64, servicePolicy *persistence.ServicePolicy) {
	strategy.handleServicePolicy(index, edge_ctrl_pb.DataState_Delete, servicePolicy)
}

func (strategy *InstantStrategy) handleServicePolicy(index uint64, action edge_ctrl_pb.DataState_Action, servicePolicy *persistence.ServicePolicy) {
	var sp *edge_ctrl_pb.DataState_ServicePolicy

	err := strategy.ae.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		sp = newServicePolicy(tx, strategy.ae, servicePolicy)
		return nil
	})

	if err != nil {
		pfxlog.Logger().WithField("id", servicePolicy.Id).WithError(err).Errorf("could not handle %s for %T", actionToName(action), servicePolicy)
		return
	}

	strategy.Apply(&edge_ctrl_pb.DataState_Event{
		Index:  index,
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_ServicePolicy{
			ServicePolicy: sp,
		},
	})
}

func (strategy *InstantStrategy) IdentityCreate(index uint64, identity *persistence.Identity) {
	strategy.handleIdentity(index, edge_ctrl_pb.DataState_Create, identity)
}

func (strategy *InstantStrategy) IdentityUpdate(index uint64, identity *persistence.Identity) {
	strategy.handleIdentity(index, edge_ctrl_pb.DataState_Update, identity)
}

func (strategy *InstantStrategy) IdentityDelete(index uint64, identity *persistence.Identity) {
	strategy.handleIdentity(index, edge_ctrl_pb.DataState_Delete, identity)
}

func (strategy *InstantStrategy) handleIdentity(index uint64, action edge_ctrl_pb.DataState_Action, identity *persistence.Identity) {
	id := newIdentity(identity)

	strategy.Apply(&edge_ctrl_pb.DataState_Event{
		Index:  index,
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_Identity{
			Identity: id,
		},
	})
}

func (strategy *InstantStrategy) ServiceCreate(index uint64, service *persistence.EdgeService) {
	strategy.handleService(index, edge_ctrl_pb.DataState_Create, service)
}

func (strategy *InstantStrategy) ServiceUpdate(index uint64, service *persistence.EdgeService) {
	strategy.handleService(index, edge_ctrl_pb.DataState_Update, service)
}

func (strategy *InstantStrategy) ServiceDelete(index uint64, service *persistence.EdgeService) {
	strategy.handleService(index, edge_ctrl_pb.DataState_Delete, service)
}

func (strategy *InstantStrategy) handleService(index uint64, action edge_ctrl_pb.DataState_Action, service *persistence.EdgeService) {
	svc := newService(service)

	strategy.Apply(&edge_ctrl_pb.DataState_Event{
		Index:  index,
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_Service{
			Service: svc,
		},
	})
}

func (strategy *InstantStrategy) handlePostureCheck(index uint64, action edge_ctrl_pb.DataState_Action, postureCheck *persistence.PostureCheck) {
	pc := newPostureCheck(postureCheck)

	strategy.Apply(&edge_ctrl_pb.DataState_Event{
		Index:  index,
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_PostureCheck{
			PostureCheck: pc,
		},
	})
}

func (strategy *InstantStrategy) PostureCheckCreate(index uint64, postureCheck *persistence.PostureCheck) {
	strategy.handlePostureCheck(index, edge_ctrl_pb.DataState_Create, postureCheck)
}

func (strategy *InstantStrategy) PostureCheckUpdate(index uint64, postureCheck *persistence.PostureCheck) {
	strategy.handlePostureCheck(index, edge_ctrl_pb.DataState_Update, postureCheck)
}

func (strategy *InstantStrategy) PostureCheckDelete(index uint64, postureCheck *persistence.PostureCheck) {
	strategy.handlePostureCheck(index, edge_ctrl_pb.DataState_Delete, postureCheck)
}

func (strategy *InstantStrategy) PeerAdded(peers []*event.ClusterPeer) {
	for _, peer := range peers {
		strategy.Apply(&edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_PublicKey{
				PublicKey: newPublicKey(peer.ServerCert[0].Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_JWTValidation}),
			},
		})
	}
}

func (strategy *InstantStrategy) CaCreate(index uint64, ca *persistence.Ca) {
	certs := nfPem.PemBytesToCertificates([]byte(ca.CertPem))

	if len(certs) > 0 {
		strategy.handlePublicKey(index, edge_ctrl_pb.DataState_Create, newPublicKey(certs[0].Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation}))
	}
}

func (strategy *InstantStrategy) CaUpdate(index uint64, ca *persistence.Ca) {
	certs := nfPem.PemBytesToCertificates([]byte(ca.CertPem))

	if len(certs) > 0 {
		strategy.handlePublicKey(index, edge_ctrl_pb.DataState_Update, newPublicKey(certs[0].Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation}))
	}
}

func (strategy *InstantStrategy) CaDelete(index uint64, ca *persistence.Ca) {
	certs := nfPem.PemBytesToCertificates([]byte(ca.CertPem))

	if len(certs) > 0 {
		strategy.handlePublicKey(index, edge_ctrl_pb.DataState_Delete, newPublicKey(certs[0].Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation}))
	}
}

func (strategy *InstantStrategy) RevocationCreate(index uint64, revocation *persistence.Revocation) {
	strategy.handleRevocation(index, edge_ctrl_pb.DataState_Create, revocation)
}

func (strategy *InstantStrategy) RevocationUpdate(index uint64, revocation *persistence.Revocation) {
	strategy.handleRevocation(index, edge_ctrl_pb.DataState_Create, revocation)
}

func (strategy *InstantStrategy) RevocationDelete(index uint64, revocation *persistence.Revocation) {
	strategy.handleRevocation(index, edge_ctrl_pb.DataState_Create, revocation)
}

func (strategy *InstantStrategy) handlePublicKey(index uint64, action edge_ctrl_pb.DataState_Action, publicKey *edge_ctrl_pb.DataState_PublicKey) {
	strategy.Apply(&edge_ctrl_pb.DataState_Event{
		Index:  index,
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_PublicKey{
			PublicKey: publicKey,
		},
	})
}

func (strategy *InstantStrategy) sendDataStatEvent(rtx *RouterSender, stateEvent *edge_ctrl_pb.DataState_Event) error {
	content, err := proto.Marshal(stateEvent)

	if err != nil {
		return err
	}

	msg := channel.NewMessage(env.DataStateEventType, content)

	return rtx.Send(msg)

}

func (strategy *InstantStrategy) sendDataState(rtx *RouterSender, state *edge_ctrl_pb.DataState) error {
	content, err := proto.Marshal(state)

	if err != nil {
		return err
	}

	msg := channel.NewMessage(env.DataStateType, content)

	return rtx.Send(msg)
}

func (strategy *InstantStrategy) handleRevocation(index uint64, action edge_ctrl_pb.DataState_Action, revocation *persistence.Revocation) {
	strategy.Apply(&edge_ctrl_pb.DataState_Event{
		Index:  index,
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_Revocation{
			Revocation: &edge_ctrl_pb.DataState_Revocation{
				Id:        revocation.Id,
				ExpiresAt: timestamppb.New(revocation.ExpiresAt),
			},
		},
	})
}

type IndexProvider interface {
	// NextIndex provides an index for the supplied MutateContext.
	NextIndex(ctx boltz.MutateContext) (uint64, error)

	// CurrentIndex provides the current index
	CurrentIndex() uint64
}

type NonHaIndexProvider struct {
	ae          *env.AppEnv
	initialLoad sync.Once
	index       uint64

	lock sync.Mutex
}

func (p *NonHaIndexProvider) load() {
	p.lock.Lock()
	defer p.lock.Unlock()

	ctx := boltz.NewMutateContext(context.Background())
	err := p.ae.GetDbProvider().GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		zdb, err := ctx.Tx().CreateBucketIfNotExists([]byte(ZdbKey))

		if err != nil {
			return err
		}

		indexBytes := zdb.Get([]byte(ZdbIndexKey))

		if len(indexBytes) == 8 {
			p.index = binary.BigEndian.Uint64(indexBytes)
		} else {
			p.index = 0
			indexBytes = make([]byte, 8)
			binary.BigEndian.PutUint64(indexBytes, p.index)
			_ = zdb.Put([]byte(ZdbIndexKey), indexBytes)
		}

		return nil
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Fatal("could not load initial index")
	}
}

func (p *NonHaIndexProvider) NextIndex(_ boltz.MutateContext) (uint64, error) {
	p.initialLoad.Do(p.load)

	p.lock.Lock()
	defer p.lock.Unlock()

	ctx := boltz.NewMutateContext(context.Background())
	err := p.ae.GetDbProvider().GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		zdb := ctx.Tx().Bucket([]byte(ZdbKey))

		newIndex := p.index + 1

		indexBytes := make([]byte, 8) // Create a byte slice with 8 bytes
		binary.BigEndian.PutUint64(indexBytes, newIndex)
		err := zdb.Put([]byte(ZdbIndexKey), indexBytes)

		if err != nil {
			return err
		}

		p.index = newIndex
		return nil
	})

	if err != nil {
		return 0, err
	}

	return p.index, nil
}

func (p *NonHaIndexProvider) CurrentIndex() uint64 {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.index
}

type RaftIndexProvider struct {
	index uint64
	lock  sync.Mutex
}

func (p *RaftIndexProvider) NextIndex(ctx boltz.MutateContext) (uint64, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	changeCtx := change.FromContext(ctx.Context())
	if changeCtx != nil {
		p.index = changeCtx.RaftIndex
		return changeCtx.RaftIndex, nil
	}

	return 0, errors.New("could not locate raft index from MutateContext")
}

func (p *RaftIndexProvider) CurrentIndex() uint64 {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.index
}

// constraintToIndexedEvents allows constraint events to be converted to events that provide the end state of an
// entity and the index it was modified on. In non-HA scenarios, the index should be a locally tracked integer.
// In HA scenarios, it should be the index from the event store. Constraint events are used as they provide
// access to the HA raft index.
type constraintToIndexedEvents[E boltz.Entity] struct {
	indexProvider IndexProvider

	createHandler func(uint64, E)
	updateHandler func(uint64, E)
	deleteHandler func(uint64, E)
}

// ProcessPreCommit is a pass through to satisfy interface requirements.
func (h *constraintToIndexedEvents[E]) ProcessPreCommit(_ *boltz.EntityChangeState[E]) error {
	return nil
}

func (h *constraintToIndexedEvents[E]) ProcessPostCommit(state *boltz.EntityChangeState[E]) {
	switch state.ChangeType {
	case boltz.EntityCreated:
		if h.createHandler != nil {
			index, err := h.indexProvider.NextIndex(state.Ctx)

			if err != nil {
				pfxlog.Logger().WithError(err).Errorf("could not process post commit create for %T, could not aquire index", state.FinalState)
				return
			}

			h.createHandler(index, state.FinalState)
		}
	case boltz.EntityUpdated:
		if h.updateHandler != nil {
			index, err := h.indexProvider.NextIndex(state.Ctx)

			if err != nil {
				pfxlog.Logger().WithError(err).Errorf("could not process post commit update for %T, could not aquire index", state.FinalState)
				return
			}

			h.updateHandler(index, state.FinalState)
		}
	case boltz.EntityDeleted:
		if h.deleteHandler != nil {
			index, err := h.indexProvider.NextIndex(state.Ctx)

			if err != nil {
				pfxlog.Logger().WithError(err).Errorf("could not process post commit delete for %T, could not aquire index", state.FinalState)
				return
			}

			//initial state for delete has the actual value, final state is nil
			h.deleteHandler(index, state.InitialState)
		}
	}
}
