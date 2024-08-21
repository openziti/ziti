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
	"crypto/tls"
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
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/handler_edge_ctrl"
	"github.com/openziti/ziti/controller/model"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

const (
	RouterSyncStrategyInstant env.RouterSyncStrategyType = "instant"
	ZdbIndexKey               string                     = "index"
	ZdbKey                    string                     = "zdb"
)

var _ env.RouterSyncStrategy = (*InstantStrategy)(nil)

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

	changeSetLock sync.Mutex
	changeSets    map[uint64]*edge_ctrl_pb.DataState_ChangeSet
}

func (strategy *InstantStrategy) AddPublicKey(cert *tls.Certificate) {
	publicKey := newPublicKey(cert.Certificate[0], edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation, edge_ctrl_pb.DataState_PublicKey_JWTValidation})
	newModel := &edge_ctrl_pb.DataState_Event_PublicKey{PublicKey: publicKey}
	newEvent := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model:  newModel,
	}

	strategy.HandlePublicKeyEvent(newEvent, newModel)
}

// Initialize implements RouterDataModelCache
func (strategy *InstantStrategy) Initialize(logSize uint64, bufferSize uint) error {
	strategy.RouterDataModel = common.NewSenderRouterDataModel(logSize, bufferSize)

	if strategy.ae.HostController.IsRaftEnabled() {
		strategy.indexProvider = &RaftIndexProvider{
			index: strategy.ae.GetHostController().GetRaftIndex(),
		}
	} else {
		strategy.indexProvider = &NonHaIndexProvider{
			ae: strategy.ae,
		}
	}

	err := strategy.BuildAll(strategy.RouterDataModel)

	if err != nil {
		return err
	}

	go strategy.handleRouterModelEvents(strategy.RouterDataModel.NewListener())

	//policy create/delete/update
	servicePolicyHandler := &constraintToIndexedEvents[*db.ServicePolicy]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.ServicePolicyCreate,
		updateHandler: strategy.ServicePolicyUpdate,
		deleteHandler: strategy.ServicePolicyDelete,
	}
	strategy.ae.GetStores().ServicePolicy.AddEntityConstraint(servicePolicyHandler)

	//identity create/delete/update
	identityHandler := &constraintToIndexedEvents[*db.Identity]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.IdentityCreate,
		updateHandler: strategy.IdentityUpdate,
		deleteHandler: strategy.IdentityDelete,
	}
	strategy.ae.GetStores().Identity.AddEntityConstraint(identityHandler)

	//posture check create/delete/update
	postureCheckHandler := &constraintToIndexedEvents[*db.PostureCheck]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.PostureCheckCreate,
		updateHandler: strategy.PostureCheckUpdate,
		deleteHandler: strategy.PostureCheckDelete,
	}
	strategy.ae.GetStores().PostureCheck.AddEntityConstraint(postureCheckHandler)

	//configType create/delete/update
	configTypeHandler := &constraintToIndexedEvents[*db.ConfigType]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.ConfigTypeCreate,
		updateHandler: strategy.ConfigTypeUpdate,
		deleteHandler: strategy.ConfigTypeDelete,
	}
	strategy.ae.GetStores().ConfigType.AddEntityConstraint(configTypeHandler)

	//config create/delete/update
	configHandler := &constraintToIndexedEvents[*db.Config]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.ConfigCreate,
		updateHandler: strategy.ConfigUpdate,
		deleteHandler: strategy.ConfigDelete,
	}
	strategy.ae.GetStores().Config.AddEntityConstraint(configHandler)

	//service create/delete/update
	serviceHandler := &constraintToIndexedEvents[*db.EdgeService]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.ServiceCreate,
		updateHandler: strategy.ServiceUpdate,
		deleteHandler: strategy.ServiceDelete,
	}
	strategy.ae.GetStores().EdgeService.AddEntityConstraint(serviceHandler)

	//ca create/delete/update
	caHandler := &constraintToIndexedEvents[*db.Ca]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.CaCreate,
		updateHandler: strategy.CaUpdate,
		deleteHandler: strategy.CaDelete,
	}
	strategy.ae.GetStores().Ca.AddEntityConstraint(caHandler)

	//revocation create/delete/update
	revocationHandler := &constraintToIndexedEvents[*db.Revocation]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.RevocationCreate,
		updateHandler: strategy.RevocationUpdate,
		deleteHandler: strategy.RevocationDelete,
	}
	strategy.ae.GetStores().Revocation.AddEntityConstraint(revocationHandler)

	controllerHandler := &constraintToIndexedEvents[*db.Controller]{
		indexProvider: strategy.indexProvider,
		createHandler: strategy.ControllerCreate,
		updateHandler: strategy.ControllerUpdate,
	}

	strategy.ae.GetStores().Controller.AddEntityConstraint(controllerHandler)
	strategy.ae.GetDb().AddTxCompleteListener(strategy.completeChangeSet)

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
		changeSets:               map[uint64]*edge_ctrl_pb.DataState_ChangeSet{},
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

	ae.GetHostController().GetNetwork().AddInspectTarget(strategy.inspect)

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

func (strategy *InstantStrategy) RouterConnected(edgeRouter *model.EdgeRouter, router *model.Router) {
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

func (strategy *InstantStrategy) RouterDisconnected(router *model.Router) {
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

func (strategy *InstantStrategy) ApiSessionAdded(apiSession *db.ApiSession) {
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

func (strategy *InstantStrategy) ApiSessionUpdated(apiSession *db.ApiSession, _ *db.ApiSessionCertificate) {
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

func (strategy *InstantStrategy) ApiSessionDeleted(apiSession *db.ApiSession) {
	sessionRemoved := &edge_ctrl_pb.ApiSessionRemoved{
		Tokens: []string{apiSession.Token},
	}

	strategy.rtxMap.Range(func(rtx *RouterSender) {
		content, _ := proto.Marshal(sessionRemoved)
		msg := channel.NewMessage(env.ApiSessionRemovedType, content)
		_ = rtx.Send(msg)
	})
}

func (strategy *InstantStrategy) SessionDeleted(session *db.Session) {
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
	msg := channel.NewMessage(env.ServerHelloType, buf)

	msg.PutBoolHeader(int32(edge_ctrl_pb.Header_RouterDataModel), true)

	if err = msg.WithTimeout(strategy.HelloSendTimeout).Send(rtx.Router.Control); err != nil {
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

func (strategy *InstantStrategy) ReceiveResync(routerId string, _ *edge_ctrl_pb.RequestClientReSync) {
	rtx := strategy.rtxMap.Get(routerId)

	if rtx == nil {
		routerName := "<unable to retrieve>"
		if router, _ := strategy.ae.Managers.Router.Read(routerId); router != nil {
			routerName = router.Name
		}
		pfxlog.Logger().
			WithField("strategy", strategy.Type()).
			WithField("routerId", routerId).
			WithField("routerName", routerName).
			Error("received resync from router that is currently not tracked by the strategy, dropping resync")
		return
	}

	rtx.SetSyncStatus(env.RouterSyncResyncWait)

	rtx.logger().WithField("strategy", strategy.Type()).Info("received resync from router, queuing")

	rtx.RouterModelIndex = nil

	strategy.queueClientHello(rtx)
}

func (strategy *InstantStrategy) queueClientHello(rtx *RouterSender) {
	select {
	case strategy.receivedClientHelloQueue <- rtx:
		return
	default:
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	go func() {
		for {
			if ch := rtx.Router.Control; ch == nil || ch.IsClosed() {
				return
			}

			select {
			case strategy.receivedClientHelloQueue <- rtx:
				return
			case <-strategy.stopNotify:
				return
			case <-ticker.C:
			}
		}
	}()
}

func (strategy *InstantStrategy) ReceiveClientHello(routerId string, msg *channel.Message, respHello *edge_ctrl_pb.ClientHello) {
	rtx := strategy.rtxMap.Get(routerId)

	if rtx == nil {
		routerName := "<unable to retrieve>"
		if router, _ := strategy.ae.Managers.Router.Read(routerId); router != nil {
			routerName = router.Name
		}
		pfxlog.Logger().
			WithField("strategy", strategy.Type()).
			WithField("routerId", routerId).
			WithField("routerName", routerName).
			Error("received hello from router that is currently not tracked by the strategy, dropping hello")
		return
	}
	rtx.SetSyncStatus(env.RouterSyncHelloWait)

	logger := rtx.logger().WithField("strategy", strategy.Type()).
		WithField("protocols", respHello.Protocols).
		WithField("protocolPorts", respHello.ProtocolPorts).
		WithField("listeners", respHello.Listeners).
		WithField("data", respHello.Data).
		WithField("version", rtx.Router.VersionInfo.Version).
		WithField("revision", rtx.Router.VersionInfo.Revision).
		WithField("buildDate", rtx.Router.VersionInfo.BuildDate).
		WithField("os", rtx.Router.VersionInfo.OS).
		WithField("arch", rtx.Router.VersionInfo.Arch)

	if supported, ok := msg.Headers.GetBoolHeader(int32(edge_ctrl_pb.Header_RouterDataModel)); ok && supported {
		rtx.SupportsRouterModel = true

		if index, ok := msg.Headers.GetUint64Header(int32(edge_ctrl_pb.Header_RouterDataModelIndex)); ok {
			rtx.RouterModelIndex = &index
		}
	}

	protocols := map[string]string{}

	if len(respHello.Listeners) > 0 {
		for _, listener := range respHello.Listeners {
			protocols[listener.Advertise.Protocol] = fmt.Sprintf("%s://%s:%d", listener.Advertise.Protocol, listener.Advertise.Hostname, listener.Advertise.Port)
		}
	} else if respHello.Hostname != "" {
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
	rtx.SetVersionInfo(*rtx.Router.VersionInfo)

	serverVersion := build.GetBuildInfo().Version()
	logger.Infof("edge router sent hello with version [%s] to controller with version [%s]", respHello.Version, serverVersion)
	strategy.queueClientHello(rtx)
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
	err := strategy.ae.GetDb().View(func(tx *bbolt.Tx) error {
		var apiSessions []*edge_ctrl_pb.ApiSession

		state := &InstantSyncState{
			Id:       cuid.New(),
			IsLast:   true,
			Sequence: 0,
		}

		for cursor := strategy.ae.GetStores().ApiSession.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
			current := cursor.Current()

			apiSession, err := strategy.ae.GetStores().ApiSession.LoadById(tx, string(current))

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
					err = strategy.sendDataStateChangeSet(rtx, curEvent)
					if err != nil {
						pfxlog.Logger().WithError(err).
							WithField("eventIndex", curEvent.Index).
							WithField("evenType", reflect.TypeOf(curEvent).String()).
							WithField("eventIsSynthetic", curEvent.IsSynthetic).
							Error("could not send data state event")
					}
				}

				var pks []*edge_ctrl_pb.DataState_PublicKey
				strategy.RouterDataModel.PublicKeys.IterCb(func(_ string, v *edge_ctrl_pb.DataState_PublicKey) {
					pks = append(pks, v)
				})

				for _, pk := range pks {
					peerEvent := &edge_ctrl_pb.DataState_Event{
						IsSynthetic: true,
						Action:      edge_ctrl_pb.DataState_Create,
						Model: &edge_ctrl_pb.DataState_Event_PublicKey{
							PublicKey: newPublicKey(pk.Data, pk.Format, pk.Usages),
						},
					}

					changeSet := &edge_ctrl_pb.DataState_ChangeSet{
						Changes: []*edge_ctrl_pb.DataState_Event{peerEvent},
					}

					err = strategy.sendDataStateChangeSet(rtx, changeSet)

					if err != nil {
						pfxlog.Logger().WithError(err).
							WithField("evenType", reflect.TypeOf(peerEvent).String()).
							WithField("eventAction", peerEvent.Action).
							WithField("eventIsSynthetic", peerEvent.IsSynthetic).
							Error("could not send data state event for peers")
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

func (strategy *InstantStrategy) handleRouterModelEvents(eventChannel <-chan *edge_ctrl_pb.DataState_ChangeSet) {
	for {
		select {
		case newEvent := <-eventChannel:
			strategy.rtxMap.Range(func(rtx *RouterSender) {

				if !rtx.SupportsRouterModel {
					return
				}

				err := strategy.sendDataStateChangeSet(rtx, newEvent)

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

func (strategy *InstantStrategy) BuildServicePolicies(tx *bbolt.Tx, rdm *common.RouterDataModel) error {
	for cursor := strategy.ae.GetStores().ServicePolicy.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		currentBytes := cursor.Current()
		currentId := string(currentBytes)

		storeModel, err := strategy.ae.GetStores().ServicePolicy.LoadById(tx, currentId)

		if err != nil {
			return err
		}

		servicePolicy := newServicePolicy(storeModel)

		newModel := &edge_ctrl_pb.DataState_Event_ServicePolicy{ServicePolicy: servicePolicy}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}
		rdm.HandleServicePolicyEvent(newEvent, newModel)

		result := strategy.ae.GetManagers().ServicePolicy.ListAssociatedIds(tx, storeModel.Id)

		addServicesEvent := &edge_ctrl_pb.DataState_ServicePolicyChange{
			PolicyId:          currentId,
			RelatedEntityIds:  result.ServiceIds,
			RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService,
			Add:               true,
		}

		rdm.HandleServicePolicyChange(strategy.indexProvider.CurrentIndex(), addServicesEvent)

		addIdentitiesEvent := &edge_ctrl_pb.DataState_ServicePolicyChange{
			PolicyId:          currentId,
			RelatedEntityIds:  result.IdentityIds,
			RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
			Add:               true,
		}
		rdm.HandleServicePolicyChange(strategy.indexProvider.CurrentIndex(), addIdentitiesEvent)

		addPostureChecksEvent := &edge_ctrl_pb.DataState_ServicePolicyChange{
			PolicyId:          currentId,
			RelatedEntityIds:  result.PostureCheckIds,
			RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck,
			Add:               true,
		}
		rdm.HandleServicePolicyChange(strategy.indexProvider.CurrentIndex(), addPostureChecksEvent)
	}

	return nil
}

func (strategy *InstantStrategy) BuildPublicKeys(tx *bbolt.Tx, rdm *common.RouterDataModel) error {
	serverTls := strategy.ae.HostController.Identity().ServerCert()

	newModel := &edge_ctrl_pb.DataState_Event_PublicKey{PublicKey: newPublicKey(serverTls[0].Certificate[0], edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_JWTValidation, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation})}
	newEvent := &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model:  newModel,
	}
	strategy.HandlePublicKeyEvent(newEvent, newModel)

	for cursor := strategy.ae.GetStores().Controller.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		currentBytes := cursor.Current()
		currentId := string(currentBytes)

		storeModel, err := strategy.ae.GetStores().Controller.LoadById(tx, currentId)

		if err != nil {
			return err
		}
		certs := nfPem.PemStringToCertificates(storeModel.CertPem)

		newModel := &edge_ctrl_pb.DataState_Event_PublicKey{PublicKey: newPublicKey(certs[0].Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_JWTValidation, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation})}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}
		strategy.HandlePublicKeyEvent(newEvent, newModel)
	}

	caPEMs := strategy.ae.GetConfig().Edge.CaPems()
	caCerts := nfPem.PemBytesToCertificates(caPEMs)

	for _, caCert := range caCerts {
		publicKey := newPublicKey(caCert.Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_JWTValidation, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation})
		newModel := &edge_ctrl_pb.DataState_Event_PublicKey{PublicKey: publicKey}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}

		strategy.HandlePublicKeyEvent(newEvent, newModel)
	}

	for cursor := strategy.ae.GetStores().Ca.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		currentBytes := cursor.Current()
		currentId := string(currentBytes)

		ca, err := strategy.ae.GetStores().Ca.LoadById(tx, currentId)

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
		rdm.HandlePublicKeyEvent(newEvent, newModel)
	}

	return nil
}

func (strategy *InstantStrategy) BuildAll(rdm *common.RouterDataModel) error {
	err := strategy.ae.GetDb().View(func(tx *bbolt.Tx) error {
		index := strategy.indexProvider.CurrentIndex()
		if err := strategy.BuildConfigTypes(index, tx, rdm); err != nil {
			return err
		}

		if err := strategy.BuildConfigs(index, tx, rdm); err != nil {
			return err
		}

		if err := strategy.BuildIdentities(index, tx, rdm); err != nil {
			return err
		}

		if err := strategy.BuildServices(index, tx, rdm); err != nil {
			return err
		}

		if err := strategy.BuildPostureChecks(index, tx, rdm); err != nil {
			return err
		}

		if err := strategy.BuildServicePolicies(tx, rdm); err != nil {
			return err
		}

		if err := strategy.BuildPublicKeys(tx, rdm); err != nil {
			return err
		}

		rdm.SetCurrentIndex(strategy.indexProvider.CurrentIndex())

		return nil
	})

	return err
}

func (strategy *InstantStrategy) BuildConfigTypes(index uint64, tx *bbolt.Tx, rdm *common.RouterDataModel) error {
	for cursor := strategy.ae.GetStores().ConfigType.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		currentBytes := cursor.Current()
		currentId := string(currentBytes)

		configType, err := newConfigTypeById(tx, strategy.ae, currentId)

		if err != nil {
			return err
		}

		newModel := &edge_ctrl_pb.DataState_Event_ConfigType{ConfigType: configType}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}
		rdm.HandleConfigTypeEvent(index, newEvent, newModel)
	}

	return nil
}

func (strategy *InstantStrategy) ValidateConfigTypes(tx *bbolt.Tx, rdm *common.RouterDataModel) []error {
	return ValidateType(tx, strategy.ae.GetStores().ConfigType, rdm.ConfigTypes, func(t *db.ConfigType, v *common.ConfigType) []error {
		var result []error
		result = diffVals("config type", t.Id, "name", t.Name, v.Name, result)
		return result
	})
}

func (strategy *InstantStrategy) ValidateConfigs(tx *bbolt.Tx, rdm *common.RouterDataModel) []error {
	return ValidateType(tx, strategy.ae.GetStores().Config, rdm.Configs, func(t *db.Config, v *common.Config) []error {
		var result []error
		result = diffVals("config", t.Id, "name", t.Name, v.Name, result)

		dataJson, err := json.Marshal(t.Data)
		if err != nil {
			result = append(result, err)
			return result
		}
		result = diffVals("config", t.Id, "data", string(dataJson), v.DataJson, result)
		return result
	})
}

func (strategy *InstantStrategy) ValidateIdentities(tx *bbolt.Tx, rdm *common.RouterDataModel) []error {
	return ValidateType(tx, strategy.ae.GetStores().Identity, rdm.Identities, func(t *db.Identity, v *common.Identity) []error {
		var result []error

		serviceConfigs := v.GetServiceConfigsAsMap()

		result = diffVals("identity", t.Id, "name", t.Name, v.Name, result)
		result = diffVals("identity", t.Id, "default hosting precedence", byte(t.DefaultHostingPrecedence), byte(v.DefaultHostingPrecedence), result)
		result = diffVals("identity", t.Id, "default hosting cost", uint32(t.DefaultHostingCost), v.DefaultHostingCost, result)
		result = diffJson("identity", t.Id, "service configs", t.ServiceConfigs, serviceConfigs, result)
		result = diffJson("identity", t.Id, "app data", t.AppData, v.AppDataJson, result)
		result = diffJson("identity", t.Id, "service hosting costs", t.ServiceHostingCosts, v.ServiceHostingCosts, result)
		result = diffJson("identity", t.Id, "service hosting precedences", t.ServiceHostingPrecedences, v.ServiceHostingPrecedences, result)

		policyList := strategy.ae.GetStores().Identity.GetRelatedEntitiesIdList(tx, t.Id, db.EntityTypeServicePolicies)
		policySet := genext.SliceToSet(policyList)
		result = diffSets("identity", t.Id, "service policy", policySet, v.ServicePolicies, result)

		return result
	})
}

func (strategy *InstantStrategy) ValidateServices(tx *bbolt.Tx, rdm *common.RouterDataModel) []error {
	return ValidateType(tx, strategy.ae.GetStores().EdgeService, rdm.Services, func(t *db.EdgeService, v *common.Service) []error {
		var result []error
		result = diffVals("service", t.Id, "name", t.Name, v.Name, result)
		result = diffVals("service", t.Id, "encryption required", t.EncryptionRequired, v.EncryptionRequired, result)
		result = diffJson("service", t.Id, "configs", t.Configs, v.Configs, result)
		return result
	})
}

func (strategy *InstantStrategy) ValidatePostureChecks(tx *bbolt.Tx, rdm *common.RouterDataModel) []error {
	return ValidateType(tx, strategy.ae.GetStores().PostureCheck, rdm.PostureChecks, func(t *db.PostureCheck, v *common.PostureCheck) []error {
		var result []error
		result = diffVals("posture check", t.Id, "name", t.Name, v.Name, result)
		result = diffVals("posture check", t.Id, "type", t.TypeId, v.TypeId, result)

		strToList := func(v string) []string {
			if v == "" {
				return nil
			}
			return []string{v}
		}

		switch subType := t.SubType.(type) {
		case *db.PostureCheckMacAddresses:
			if rdmSubType, ok := v.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_Mac_); ok && rdmSubType.Mac != nil {
				result = diffJson("posture check", t.Id, "mac addresses", subType.MacAddresses, rdmSubType.Mac.MacAddresses, result)
			} else {
				result = append(result, fmt.Errorf("for posture check %s, sub type not mac address, rather: %T", t.Id, v.Subtype))
			}
		case *db.PostureCheckMfa:
			if rdmSubType, ok := v.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_Mfa_); ok && rdmSubType.Mfa != nil {
				result = diffVals("posture check", t.Id, "mfa ignore legacy endpoints", subType.IgnoreLegacyEndpoints, rdmSubType.Mfa.IgnoreLegacyEndpoints, result)
				result = diffVals("posture check", t.Id, "mfa prompt on wake", subType.PromptOnWake, rdmSubType.Mfa.PromptOnWake, result)
				result = diffVals("posture check", t.Id, "mfa prompt on unlock", subType.PromptOnUnlock, rdmSubType.Mfa.PromptOnUnlock, result)
				result = diffVals("posture check", t.Id, "mfa timeout seconds", subType.TimeoutSeconds, rdmSubType.Mfa.TimeoutSeconds, result)
			} else {
				result = append(result, fmt.Errorf("for posture check %s, sub type not mfa, rather: %T", t.Id, v.Subtype))
			}
		case *db.PostureCheckOperatingSystem:
			if rdmSubType, ok := v.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_OsList_); ok && rdmSubType.OsList != nil {
				result = diffVals("posture check", t.Id, "os list len", len(subType.OperatingSystems), len(rdmSubType.OsList.OsList), result)
				if len(subType.OperatingSystems) == len(rdmSubType.OsList.OsList) {
					for idx, os := range subType.OperatingSystems {
						rdmOs := rdmSubType.OsList.OsList[idx]
						if os.OsType != rdmOs.OsType {
							result = append(result, fmt.Errorf("for posture check %s, os type at %d doesn't match rather: %s != %xs", t.Id, idx, os.OsType, rdmOs.OsType))
						}
						result = diffJson("posture check", t.Id, fmt.Sprintf("os %d versions", idx), os.OsVersions, rdmOs.OsVersions, result)
					}
				}
			} else {
				result = append(result, fmt.Errorf("for posture check %s, sub type not os list, rather: %T", t.Id, v.Subtype))
			}
		case *db.PostureCheckProcess:
			if rdmSubType, ok := v.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_Process_); ok && rdmSubType.Process != nil {
				result = diffJson("posture check", t.Id, "process fingerprints", strToList(subType.Fingerprint), rdmSubType.Process.Fingerprints, result)
				result = diffJson("posture check", t.Id, "process hashes", subType.Hashes, rdmSubType.Process.Hashes, result)
				result = diffVals("posture check", t.Id, "process path", subType.Path, rdmSubType.Process.Path, result)
				result = diffVals("posture check", t.Id, "process operating system", subType.OperatingSystem, rdmSubType.Process.OsType, result)
			} else {
				result = append(result, fmt.Errorf("for posture check %s, sub type not process, rather: %T", t.Id, v.Subtype))
			}
		case *db.PostureCheckProcessMulti:
			if rdmSubType, ok := v.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_ProcessMulti_); ok && rdmSubType.ProcessMulti != nil {
				result = diffVals("posture check", t.Id, "process multi semantic", subType.Semantic, rdmSubType.ProcessMulti.Semantic, result)
				result = diffVals("posture check", t.Id, "process multi list len", len(subType.Processes), len(rdmSubType.ProcessMulti.Processes), result)
				if len(subType.Processes) == len(rdmSubType.ProcessMulti.Processes) {
					for idx, process := range subType.Processes {
						rdmProcess := rdmSubType.ProcessMulti.Processes[idx]
						result = diffJson("posture check", t.Id, fmt.Sprintf("process %d fingerprint", idx), process.SignerFingerprints, rdmProcess.Fingerprints, result)
						result = diffJson("posture check", t.Id, fmt.Sprintf("process %d hashes", idx), process.Hashes, rdmProcess.Hashes, result)
						result = diffVals("posture check", t.Id, fmt.Sprintf("process %d path", idx), process.Path, rdmProcess.Path, result)
						result = diffVals("posture check", t.Id, fmt.Sprintf("process %d os type", idx), process.OsType, rdmProcess.OsType, result)
					}
				}
			} else {
				result = append(result, fmt.Errorf("for posture check %s, sub type not process multi, rather: %T", t.Id, v.Subtype))
			}
		}

		return result
	})
}

func (strategy *InstantStrategy) ValidateServicePolicies(tx *bbolt.Tx, rdm *common.RouterDataModel) []error {
	return ValidateType(tx, strategy.ae.GetStores().ServicePolicy, rdm.ServicePolicies, func(t *db.ServicePolicy, v *common.ServicePolicy) []error {
		var result []error

		result = diffVals("service policy", t.Id, "name", t.Name, v.Name, result)
		result = diffVals("service policy", t.Id, "policy type", t.PolicyType.Id(), int32(v.PolicyType), result)

		policyList := strategy.ae.GetStores().ServicePolicy.GetRelatedEntitiesIdList(tx, t.Id, db.EntityTypeServices)
		policySet := genext.SliceToSet(policyList)
		result = diffSets("service policy", t.Id, "service", policySet, v.Services, result)

		policyList = strategy.ae.GetStores().ServicePolicy.GetRelatedEntitiesIdList(tx, t.Id, db.EntityTypePostureChecks)
		policySet = genext.SliceToSet(policyList)
		result = diffSets("service policy", t.Id, "posture check", policySet, v.PostureChecks, result)

		return result
	})
}

func diffVals[T comparable](entityType, id, field string, a, b T, errors []error) []error {
	if a != b {
		return append(errors, fmt.Errorf("for %s %s: %s %v do not match rdm value: %v", entityType, id, field, a, b))
	}
	return errors
}

func diffJson(entityType, id, field string, a interface{}, b interface{}, errors []error) []error {
	orig := ""
	rdm := ""

	if a != nil {
		origBytes, err := json.Marshal(a)
		if err != nil {
			return append(errors, err)
		}
		orig = string(origBytes)
	}

	if b != nil {
		if bytes, ok := b.([]byte); ok {
			rdm = string(bytes)
		} else {
			rdmBytes, err := json.Marshal(b)
			if err != nil {
				return append(errors, err)
			}
			rdm = string(rdmBytes)
		}
	}

	if orig != rdm {
		return append(errors, fmt.Errorf("for %s %s: %s %s do not match rdm value: %s", entityType, id, field, orig, rdm))
	}
	return errors
}

func diffSets(entityType, id, field string, a, b map[string]struct{}, result []error) []error {
	result = diffVals(entityType, id, field+" count", len(a), len(b), result)

	for entityId := range a {
		_, found := b[entityId]
		result = diffVals(entityType, id, fmt.Sprintf("has %s %s", field, entityId), true, found, result)
	}

	for entityId := range b {
		_, found := a[entityId]
		result = diffVals(entityType, id, fmt.Sprintf("has %s %s", field, entityId), found, true, result)
	}

	return result
}

func ValidateType[T boltz.ExtEntity, V any](tx *bbolt.Tx, store db.Store[T], m cmap.ConcurrentMap[string, V], checkF func(T, V) []error) []error {
	var result []error
	entities := common.CloneMap(m)
	for cursor := store.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		entity, err := store.LoadById(tx, string(cursor.Current()))
		if err != nil {
			result = append(result, err)
			continue
		}
		rdmEntity, found := entities.Get(entity.GetId())
		if !found {
			result = append(result, fmt.Errorf("no %s found with id: %s",
				entity.GetEntityType(), entity.GetId()))
			continue
		}

		entities.Remove(entity.GetId())

		if errs := checkF(entity, rdmEntity); len(errs) > 0 {
			result = append(result, errs...)
		}
	}

	return result
}

func (strategy *InstantStrategy) BuildConfigs(index uint64, tx *bbolt.Tx, rdm *common.RouterDataModel) error {
	for cursor := strategy.ae.GetStores().Config.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		currentBytes := cursor.Current()
		currentId := string(currentBytes)

		config, err := newConfigById(tx, strategy.ae, currentId)

		if err != nil {
			return err
		}

		newModel := &edge_ctrl_pb.DataState_Event_Config{Config: config}
		newEvent := &edge_ctrl_pb.DataState_Event{
			Action: edge_ctrl_pb.DataState_Create,
			Model:  newModel,
		}
		rdm.HandleConfigEvent(index, newEvent, newModel)
	}

	return nil
}

func (strategy *InstantStrategy) BuildIdentities(index uint64, tx *bbolt.Tx, rdm *common.RouterDataModel) error {
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
		rdm.HandleIdentityEvent(index, newEvent, newModel)
	}

	return nil
}

func (strategy *InstantStrategy) BuildServices(index uint64, tx *bbolt.Tx, rdm *common.RouterDataModel) error {
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
		rdm.HandleServiceEvent(index, newEvent, newModel)
	}

	return nil
}

func (strategy *InstantStrategy) BuildPostureChecks(index uint64, tx *bbolt.Tx, rdm *common.RouterDataModel) error {
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
		rdm.HandlePostureCheckEvent(index, newEvent, newModel)
	}
	return nil
}

func (strategy *InstantStrategy) Validate() []error {
	return strategy.ValidateAll(strategy.RouterDataModel)
}

func (strategy *InstantStrategy) ValidateAll(rdm *common.RouterDataModel) []error {
	var result []error
	err := strategy.ae.GetDb().View(func(tx *bbolt.Tx) error {
		if errs := strategy.ValidateConfigTypes(tx, rdm); len(errs) > 0 {
			result = append(result, errs...)
		}

		if errs := strategy.ValidateConfigs(tx, rdm); len(errs) > 0 {
			result = append(result, errs...)
		}

		if errs := strategy.ValidateIdentities(tx, rdm); len(errs) > 0 {
			result = append(result, errs...)
		}

		if errs := strategy.ValidateServices(tx, rdm); len(errs) > 0 {
			result = append(result, errs...)
		}

		if errs := strategy.ValidatePostureChecks(tx, rdm); len(errs) != 0 {
			result = append(result, errs...)
		}

		if errs := strategy.ValidateServicePolicies(tx, rdm); len(errs) != 0 {
			result = append(result, errs...)
		}

		// TODO: validate public keys

		return nil
	})

	if err != nil {
		result = append(result, err)
	}

	return result
}

func newIdentityById(tx *bbolt.Tx, ae *env.AppEnv, id string) (*edge_ctrl_pb.DataState_Identity, error) {
	identityModel, err := ae.GetStores().Identity.LoadById(tx, id)

	if err != nil {
		return nil, err
	}

	return newIdentity(identityModel), nil
}

func newIdentity(identityModel *db.Identity) *edge_ctrl_pb.DataState_Identity {
	var hostingPrecedences map[string]edge_ctrl_pb.TerminatorPrecedence
	if identityModel.ServiceHostingPrecedences != nil {
		hostingPrecedences = map[string]edge_ctrl_pb.TerminatorPrecedence{}
		for k, v := range identityModel.ServiceHostingPrecedences {
			hostingPrecedences[k] = edge_ctrl_pb.GetPrecedence(v)
		}
	}

	var hostingCosts map[string]uint32
	if identityModel.ServiceHostingCosts != nil {
		hostingCosts = map[string]uint32{}
		for k, v := range identityModel.ServiceHostingCosts {
			hostingCosts[k] = uint32(v)
		}
	}

	var appDataJson []byte
	if identityModel.AppData != nil {
		var err error
		appDataJson, err = json.Marshal(identityModel.AppData)
		if err != nil {
			pfxlog.Logger().WithError(err).Error("Failed to marshal app data")
		}
	}

	var serviceConfigsMap map[string]*edge_ctrl_pb.DataState_ServiceConfigs

	if len(identityModel.ServiceConfigs) > 0 {
		serviceConfigsMap = map[string]*edge_ctrl_pb.DataState_ServiceConfigs{}
	}

	for serviceId, configInfo := range identityModel.ServiceConfigs {
		serviceConfigs := &edge_ctrl_pb.DataState_ServiceConfigs{}
		serviceConfigs.Configs = configInfo
		serviceConfigsMap[serviceId] = serviceConfigs
	}

	return &edge_ctrl_pb.DataState_Identity{
		Id:                        identityModel.Id,
		Name:                      identityModel.Name,
		DefaultHostingPrecedence:  edge_ctrl_pb.GetPrecedence(identityModel.DefaultHostingPrecedence),
		DefaultHostingCost:        uint32(identityModel.DefaultHostingCost),
		ServiceHostingPrecedences: hostingPrecedences,
		ServiceHostingCosts:       hostingCosts,
		AppDataJson:               appDataJson,
		ServiceConfigs:            serviceConfigsMap,
		Disabled:                  identityModel.Disabled,
	}
}

func newServicePolicy(storeModel *db.ServicePolicy) *edge_ctrl_pb.DataState_ServicePolicy {
	servicePolicy := &edge_ctrl_pb.DataState_ServicePolicy{
		Id:         storeModel.Id,
		Name:       storeModel.Name,
		PolicyType: edge_ctrl_pb.PolicyType(storeModel.PolicyType.Id()),
	}

	return servicePolicy
}

func newConfigTypeById(tx *bbolt.Tx, ae *env.AppEnv, id string) (*edge_ctrl_pb.DataState_ConfigType, error) {
	storeModel, err := ae.GetStores().ConfigType.LoadById(tx, id)

	if err != nil {
		return nil, err
	}

	return newConfigType(storeModel), nil
}

func newConfigType(storeModel *db.ConfigType) *edge_ctrl_pb.DataState_ConfigType {
	return &edge_ctrl_pb.DataState_ConfigType{
		Id:   storeModel.Id,
		Name: storeModel.Name,
	}
}

func newConfigById(tx *bbolt.Tx, ae *env.AppEnv, id string) (*edge_ctrl_pb.DataState_Config, error) {
	storeModel, err := ae.GetStores().Config.LoadById(tx, id)

	if err != nil {
		return nil, err
	}

	return newConfig(storeModel)
}

func newConfig(entity *db.Config) (*edge_ctrl_pb.DataState_Config, error) {
	jsonData, err := json.Marshal(entity.Data)
	if err != nil {
		return nil, err
	}

	return &edge_ctrl_pb.DataState_Config{
		Id:       entity.Id,
		TypeId:   entity.Type,
		Name:     entity.Name,
		DataJson: string(jsonData),
	}, nil
}

func newServiceById(tx *bbolt.Tx, ae *env.AppEnv, id string) (*edge_ctrl_pb.DataState_Service, error) {
	storeModel, err := ae.GetStores().EdgeService.LoadById(tx, id)

	if err != nil {
		return nil, err
	}

	return newService(storeModel), nil
}

func newService(storeModel *db.EdgeService) *edge_ctrl_pb.DataState_Service {
	return &edge_ctrl_pb.DataState_Service{
		Id:                 storeModel.Id,
		Name:               storeModel.Name,
		EncryptionRequired: storeModel.EncryptionRequired,
		Configs:            storeModel.Configs,
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
	postureModel, err := ae.GetStores().PostureCheck.LoadById(tx, id)

	if err != nil {
		return nil, err
	}
	return newPostureCheck(postureModel), nil
}

func newPostureCheck(postureModel *db.PostureCheck) *edge_ctrl_pb.DataState_PostureCheck {
	newVal := &edge_ctrl_pb.DataState_PostureCheck{
		Id:     postureModel.Id,
		Name:   postureModel.Name,
		TypeId: postureModel.TypeId,
	}

	switch subType := postureModel.SubType.(type) {
	case *db.PostureCheckProcess:
		newVal.Subtype = &edge_ctrl_pb.DataState_PostureCheck_Process_{
			Process: &edge_ctrl_pb.DataState_PostureCheck_Process{
				OsType:       subType.OperatingSystem,
				Path:         subType.Path,
				Hashes:       subType.Hashes,
				Fingerprints: []string{subType.Fingerprint},
			},
		}
	case *db.PostureCheckProcessMulti:
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
	case *db.PostureCheckMfa:
		newVal.Subtype = &edge_ctrl_pb.DataState_PostureCheck_Mfa_{
			Mfa: &edge_ctrl_pb.DataState_PostureCheck_Mfa{
				TimeoutSeconds:        subType.TimeoutSeconds,
				PromptOnWake:          subType.PromptOnWake,
				PromptOnUnlock:        subType.PromptOnUnlock,
				IgnoreLegacyEndpoints: subType.IgnoreLegacyEndpoints,
			},
		}

	case *db.PostureCheckWindowsDomains:
		newVal.Subtype = &edge_ctrl_pb.DataState_PostureCheck_Domains_{
			Domains: &edge_ctrl_pb.DataState_PostureCheck_Domains{
				Domains: subType.Domains,
			},
		}
	case *db.PostureCheckMacAddresses:
		newVal.Subtype = &edge_ctrl_pb.DataState_PostureCheck_Mac_{
			Mac: &edge_ctrl_pb.DataState_PostureCheck_Mac{
				MacAddresses: subType.MacAddresses,
			},
		}
	case *db.PostureCheckOperatingSystem:

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

func (strategy *InstantStrategy) ServicePolicyCreate(index uint64, servicePolicy *db.ServicePolicy) {
	strategy.handleServicePolicy(index, edge_ctrl_pb.DataState_Create, servicePolicy)
}

func (strategy *InstantStrategy) ServicePolicyUpdate(index uint64, servicePolicy *db.ServicePolicy) {
	strategy.handleServicePolicy(index, edge_ctrl_pb.DataState_Update, servicePolicy)
}

func (strategy *InstantStrategy) ServicePolicyDelete(index uint64, servicePolicy *db.ServicePolicy) {
	strategy.handleServicePolicy(index, edge_ctrl_pb.DataState_Delete, servicePolicy)
}

func (strategy *InstantStrategy) handleServicePolicy(index uint64, action edge_ctrl_pb.DataState_Action, servicePolicy *db.ServicePolicy) {
	sp := newServicePolicy(servicePolicy)

	strategy.addToChangeSet(index, &edge_ctrl_pb.DataState_Event{
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_ServicePolicy{
			ServicePolicy: sp,
		},
	})
}

func (strategy *InstantStrategy) IdentityCreate(index uint64, identity *db.Identity) {
	strategy.handleIdentity(index, edge_ctrl_pb.DataState_Create, identity)
}

func (strategy *InstantStrategy) IdentityUpdate(index uint64, identity *db.Identity) {
	strategy.handleIdentity(index, edge_ctrl_pb.DataState_Update, identity)
}

func (strategy *InstantStrategy) IdentityDelete(index uint64, identity *db.Identity) {
	strategy.handleIdentity(index, edge_ctrl_pb.DataState_Delete, identity)
}

func (strategy *InstantStrategy) handleIdentity(index uint64, action edge_ctrl_pb.DataState_Action, identity *db.Identity) {
	id := newIdentity(identity)

	strategy.addToChangeSet(index, &edge_ctrl_pb.DataState_Event{
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_Identity{
			Identity: id,
		},
	})
}

func (strategy *InstantStrategy) ConfigTypeCreate(index uint64, entity *db.ConfigType) {
	strategy.handleConfigType(index, edge_ctrl_pb.DataState_Create, entity)
}

func (strategy *InstantStrategy) ConfigTypeUpdate(index uint64, entity *db.ConfigType) {
	strategy.handleConfigType(index, edge_ctrl_pb.DataState_Update, entity)
}

func (strategy *InstantStrategy) ConfigTypeDelete(index uint64, entity *db.ConfigType) {
	strategy.handleConfigType(index, edge_ctrl_pb.DataState_Delete, entity)
}

func (strategy *InstantStrategy) ConfigCreate(index uint64, entity *db.Config) {
	strategy.handleConfig(index, edge_ctrl_pb.DataState_Create, entity)
}

func (strategy *InstantStrategy) ConfigUpdate(index uint64, entity *db.Config) {
	strategy.handleConfig(index, edge_ctrl_pb.DataState_Update, entity)
}

func (strategy *InstantStrategy) ConfigDelete(index uint64, entity *db.Config) {
	strategy.handleConfig(index, edge_ctrl_pb.DataState_Delete, entity)
}

func (strategy *InstantStrategy) ServiceCreate(index uint64, service *db.EdgeService) {
	strategy.handleService(index, edge_ctrl_pb.DataState_Create, service)
}

func (strategy *InstantStrategy) ServiceUpdate(index uint64, service *db.EdgeService) {
	strategy.handleService(index, edge_ctrl_pb.DataState_Update, service)
}

func (strategy *InstantStrategy) ServiceDelete(index uint64, service *db.EdgeService) {
	strategy.handleService(index, edge_ctrl_pb.DataState_Delete, service)
}

func (strategy *InstantStrategy) handleConfigType(index uint64, action edge_ctrl_pb.DataState_Action, entity *db.ConfigType) {
	configType := newConfigType(entity)

	strategy.addToChangeSet(index, &edge_ctrl_pb.DataState_Event{
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_ConfigType{
			ConfigType: configType,
		},
	})
}

func (strategy *InstantStrategy) handleConfig(index uint64, action edge_ctrl_pb.DataState_Action, entity *db.Config) {
	config, err := newConfig(entity)

	if err != nil {
		pfxlog.Logger().WithError(err).WithField("configId", entity.Id).Error("unable to marshal config json")
		return
	}

	strategy.addToChangeSet(index, &edge_ctrl_pb.DataState_Event{
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_Config{
			Config: config,
		},
	})
}

func (strategy *InstantStrategy) handleService(index uint64, action edge_ctrl_pb.DataState_Action, service *db.EdgeService) {
	svc := newService(service)

	strategy.addToChangeSet(index, &edge_ctrl_pb.DataState_Event{
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_Service{
			Service: svc,
		},
	})
}

func (strategy *InstantStrategy) handlePostureCheck(index uint64, action edge_ctrl_pb.DataState_Action, postureCheck *db.PostureCheck) {
	pc := newPostureCheck(postureCheck)

	strategy.addToChangeSet(index, &edge_ctrl_pb.DataState_Event{
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_PostureCheck{
			PostureCheck: pc,
		},
	})
}

func (strategy *InstantStrategy) PostureCheckCreate(index uint64, postureCheck *db.PostureCheck) {
	strategy.handlePostureCheck(index, edge_ctrl_pb.DataState_Create, postureCheck)
}

func (strategy *InstantStrategy) PostureCheckUpdate(index uint64, postureCheck *db.PostureCheck) {
	strategy.handlePostureCheck(index, edge_ctrl_pb.DataState_Update, postureCheck)
}

func (strategy *InstantStrategy) PostureCheckDelete(index uint64, postureCheck *db.PostureCheck) {
	strategy.handlePostureCheck(index, edge_ctrl_pb.DataState_Delete, postureCheck)
}

func (strategy *InstantStrategy) ControllerCreate(index uint64, controller *db.Controller) {
	certs := nfPem.PemStringToCertificates(controller.CertPem)
	cert := certs[0]
	strategy.handlePublicKey(index, edge_ctrl_pb.DataState_Create, newPublicKey(cert.Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation, edge_ctrl_pb.DataState_PublicKey_JWTValidation}))
}

func (strategy *InstantStrategy) ControllerUpdate(index uint64, controller *db.Controller) {
	certs := nfPem.PemStringToCertificates(controller.CertPem)
	cert := certs[0]
	strategy.handlePublicKey(index, edge_ctrl_pb.DataState_Create, newPublicKey(cert.Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation, edge_ctrl_pb.DataState_PublicKey_JWTValidation}))
}

func (strategy *InstantStrategy) CaCreate(index uint64, ca *db.Ca) {
	certs := nfPem.PemBytesToCertificates([]byte(ca.CertPem))

	if len(certs) > 0 {
		strategy.handlePublicKey(index, edge_ctrl_pb.DataState_Create, newPublicKey(certs[0].Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation}))
	}
}

func (strategy *InstantStrategy) CaUpdate(index uint64, ca *db.Ca) {
	certs := nfPem.PemBytesToCertificates([]byte(ca.CertPem))

	if len(certs) > 0 {
		strategy.handlePublicKey(index, edge_ctrl_pb.DataState_Update, newPublicKey(certs[0].Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation}))
	}
}

func (strategy *InstantStrategy) CaDelete(index uint64, ca *db.Ca) {
	certs := nfPem.PemBytesToCertificates([]byte(ca.CertPem))

	if len(certs) > 0 {
		strategy.handlePublicKey(index, edge_ctrl_pb.DataState_Delete, newPublicKey(certs[0].Raw, edge_ctrl_pb.DataState_PublicKey_X509CertDer, []edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation}))
	}
}

func (strategy *InstantStrategy) RevocationCreate(index uint64, revocation *db.Revocation) {
	strategy.handleRevocation(index, edge_ctrl_pb.DataState_Create, revocation)
}

func (strategy *InstantStrategy) RevocationUpdate(index uint64, revocation *db.Revocation) {
	strategy.handleRevocation(index, edge_ctrl_pb.DataState_Create, revocation)
}

func (strategy *InstantStrategy) RevocationDelete(index uint64, revocation *db.Revocation) {
	strategy.handleRevocation(index, edge_ctrl_pb.DataState_Create, revocation)
}

func (strategy *InstantStrategy) handlePublicKey(index uint64, action edge_ctrl_pb.DataState_Action, publicKey *edge_ctrl_pb.DataState_PublicKey) {
	strategy.addToChangeSet(index, &edge_ctrl_pb.DataState_Event{
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_PublicKey{
			PublicKey: publicKey,
		},
	})
}

func (strategy *InstantStrategy) sendDataStateChangeSet(rtx *RouterSender, stateEvent *edge_ctrl_pb.DataState_ChangeSet) error {
	content, err := proto.Marshal(stateEvent)

	if err != nil {
		return err
	}

	msg := channel.NewMessage(env.DataStateChangeSetType, content)

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

func (strategy *InstantStrategy) handleRevocation(index uint64, action edge_ctrl_pb.DataState_Action, revocation *db.Revocation) {
	strategy.addToChangeSet(index, &edge_ctrl_pb.DataState_Event{
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_Revocation{
			Revocation: &edge_ctrl_pb.DataState_Revocation{
				Id:        revocation.Id,
				ExpiresAt: timestamppb.New(revocation.ExpiresAt),
			},
		},
	})
}

func (strategy *InstantStrategy) addToChangeSet(index uint64, event *edge_ctrl_pb.DataState_Event) {
	strategy.changeSetLock.Lock()
	defer strategy.changeSetLock.Unlock()

	changeSet, found := strategy.changeSets[index]
	if !found {
		changeSet = &edge_ctrl_pb.DataState_ChangeSet{
			Index: index,
		}
		strategy.changeSets[index] = changeSet
	}
	changeSet.Changes = append(changeSet.Changes, event)
}

func (strategy *InstantStrategy) completeChangeSet(ctx boltz.MutateContext) {
	strategy.changeSetLock.Lock()
	defer strategy.changeSetLock.Unlock()

	indexPtr := strategy.indexProvider.ContextIndex(ctx)
	if indexPtr == nil {
		return
	}
	index := *indexPtr
	changeSet := strategy.changeSets[index]

	for k := range strategy.changeSets {
		if k <= index {
			delete(strategy.changeSets, k)
		}
	}

	v := ctx.Context().Value(db.ServicePolicyEventsKey)
	if v != nil {
		policyEvents := v.([]*edge_ctrl_pb.DataState_ServicePolicyChange)
		if len(policyEvents) > 0 {
			if changeSet == nil {
				changeSet = &edge_ctrl_pb.DataState_ChangeSet{
					Index: index,
				}
			}
			for _, policyEvent := range policyEvents {
				changeSet.Changes = append(changeSet.Changes, &edge_ctrl_pb.DataState_Event{
					Action: 0,
					Model: &edge_ctrl_pb.DataState_Event_ServicePolicyChange{
						ServicePolicyChange: policyEvent,
					},
				})
			}
		}
	}

	if changeSet != nil {
		strategy.ApplyChangeSet(changeSet)
	}
}

func (strategy *InstantStrategy) inspect(val string) (bool, *string, error) {
	if val == "router-data-model" {
		rdm := strategy.RouterDataModel
		js, err := json.Marshal(rdm)
		if err != nil {
			return true, nil, err
		}
		result := string(js)
		return true, &result, nil
	}
	return false, nil, nil
}

type IndexProvider interface {
	// NextIndex provides an index for the supplied MutateContext.
	NextIndex(ctx boltz.MutateContext) (uint64, error)

	// CurrentIndex provides the current index
	CurrentIndex() uint64

	ContextIndex(ctx boltz.MutateContext) *uint64
}

type nonHahIndexKeyType string

const nonHaIndexKey = nonHahIndexKeyType("non-ha.index")

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
	err := p.ae.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
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

func (p *NonHaIndexProvider) NextIndex(ctx boltz.MutateContext) (uint64, error) {
	p.initialLoad.Do(p.load)

	p.lock.Lock()
	defer p.lock.Unlock()

	if val := ctx.Context().Value(nonHaIndexKey); val != nil {
		return val.(uint64), nil
	}

	updateCtx := boltz.NewMutateContext(context.Background())
	err := p.ae.GetDb().Update(updateCtx, func(updateCtx boltz.MutateContext) error {
		zdb := updateCtx.Tx().Bucket([]byte(ZdbKey))

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

	ctx.UpdateContext(func(ctx context.Context) context.Context {
		return context.WithValue(ctx, nonHaIndexKey, p.index)
	})

	return p.index, nil
}

func (p *NonHaIndexProvider) CurrentIndex() uint64 {
	p.initialLoad.Do(p.load)

	p.lock.Lock()
	defer p.lock.Unlock()

	return p.index
}

func (p *NonHaIndexProvider) ContextIndex(ctx boltz.MutateContext) *uint64 {
	if val := ctx.Context().Value(nonHaIndexKey); val != nil {
		result, ok := val.(uint64)
		if ok {
			return &result
		}
	}
	return nil
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

func (p *RaftIndexProvider) ContextIndex(ctx boltz.MutateContext) *uint64 {
	changeCtx := change.FromContext(ctx.Context())
	if changeCtx != nil {
		return &changeCtx.RaftIndex
	}
	return nil
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
				pfxlog.Logger().WithError(err).Errorf("could not process post commit create for %T, could not acquire index", state.FinalState)
				return
			}

			h.createHandler(index, state.FinalState)
		}
	case boltz.EntityUpdated:
		if h.updateHandler != nil {
			index, err := h.indexProvider.NextIndex(state.Ctx)

			if err != nil {
				pfxlog.Logger().WithError(err).Errorf("could not process post commit update for %T, could not acquire index", state.FinalState)
				return
			}

			h.updateHandler(index, state.FinalState)
		}
	case boltz.EntityDeleted:
		if h.deleteHandler != nil {
			index, err := h.indexProvider.NextIndex(state.Ctx)

			if err != nil {
				pfxlog.Logger().WithError(err).Errorf("could not process post commit delete for %T, could not acquire index", state.FinalState)
				return
			}

			//initial state for delete has the actual value, final state is nil
			h.deleteHandler(index, state.InitialState)
		}
	}
}
