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

package network

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/metrics/metrics_pb"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/storage/objectz"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/logcontext"
	fabricMetrics "github.com/openziti/ziti/v2/common/metrics"
	"github.com/openziti/ziti/v2/common/pb/cmd_pb"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/common/trace"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/xt"
	"github.com/sirupsen/logrus"
	"github.com/teris-io/shortid"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

const SmartRerouteAttempt = 99969996

// Config provides the values needed to create a Network instance
type Config interface {
	GetId() *identity.TokenId
	GetMetricsRegistry() metrics.Registry
	GetOptions() *config.NetworkConfig
	GetCommandDispatcher() command.Dispatcher
	GetDb() boltz.Db
	GetVersionProvider() versions.VersionProvider
	GetEventDispatcher() event.Dispatcher
	GetCloseNotify() <-chan struct{}
}

type InspectTarget func(string) (bool, *string, error)

type Network struct {
	*model.Managers
	env                    model.Env
	nodeId                 string
	options                *config.NetworkConfig
	routeSenderController  *routeSenderController
	eventDispatcher        event.Dispatcher
	traceController        trace.Controller
	routerPresenceHandlers concurrenz.CopyOnWriteSlice[model.RouterPresenceHandler]
	capabilities           []string
	closeNotify            <-chan struct{}
	watchdogCh             chan struct{}
	lock                   sync.Mutex
	strategyRegistry       xt.Registry
	lastSnapshot           time.Time
	metricsRegistry        metrics.Registry
	VersionProvider        versions.VersionProvider

	serviceEventMetrics          metrics.UsageRegistry
	serviceDialSuccessCounter    metrics.IntervalCounter
	serviceDialFailCounter       metrics.IntervalCounter
	serviceDialTimeoutCounter    metrics.IntervalCounter
	serviceDialOtherErrorCounter metrics.IntervalCounter

	serviceTerminatorTimeoutCounter           metrics.IntervalCounter
	serviceTerminatorConnectionRefusedCounter metrics.IntervalCounter
	serviceInvalidTerminatorCounter           metrics.IntervalCounter
	serviceMisconfiguredTerminatorCounter     metrics.IntervalCounter

	config Config

	Inspections       *InspectionsManager
	RouterMessaging   *RouterMessaging
	inspectionTargets concurrenz.CopyOnWriteSlice[InspectTarget]
}

func NewNetwork(config Config, env model.Env) (*Network, error) {
	metricsConfig := metrics.DefaultUsageRegistryConfig(config.GetId().Token, config.GetCloseNotify())
	if config.GetOptions().IntervalAgeThreshold != 0 {
		metricsConfig.IntervalAgeThreshold = config.GetOptions().IntervalAgeThreshold
		logrus.Infof("set interval age threshold to '%v'", config.GetOptions().IntervalAgeThreshold)
	}
	serviceEventMetrics := metrics.NewUsageRegistry(metricsConfig)

	network := &Network{
		env:                   env,
		Managers:              env.GetManagers(),
		nodeId:                config.GetId().Token,
		options:               config.GetOptions(),
		routeSenderController: newRouteSenderController(),
		eventDispatcher:       config.GetEventDispatcher(),
		traceController:       trace.NewController(config.GetCloseNotify()),
		closeNotify:           config.GetCloseNotify(),
		watchdogCh:            make(chan struct{}, 1),
		strategyRegistry:      xt.GlobalRegistry(),
		lastSnapshot:          time.Now().Add(-time.Hour),
		metricsRegistry:       config.GetMetricsRegistry(),
		VersionProvider:       config.GetVersionProvider(),

		serviceEventMetrics:          serviceEventMetrics,
		serviceDialSuccessCounter:    serviceEventMetrics.IntervalCounter("service.dial.success", time.Minute),
		serviceDialFailCounter:       serviceEventMetrics.IntervalCounter("service.dial.fail", time.Minute),
		serviceDialTimeoutCounter:    serviceEventMetrics.IntervalCounter("service.dial.timeout", time.Minute),
		serviceDialOtherErrorCounter: serviceEventMetrics.IntervalCounter("service.dial.error_other", time.Minute),

		serviceTerminatorTimeoutCounter:           serviceEventMetrics.IntervalCounter("service.dial.terminator.timeout", time.Minute),
		serviceTerminatorConnectionRefusedCounter: serviceEventMetrics.IntervalCounter("service.dial.terminator.connection_refused", time.Minute),
		serviceInvalidTerminatorCounter:           serviceEventMetrics.IntervalCounter("service.dial.terminator.invalid", time.Minute),
		serviceMisconfiguredTerminatorCounter:     serviceEventMetrics.IntervalCounter("service.dial.terminator.misconfigured", time.Minute),

		config: config,
	}

	env.GetManagers().Command.Decoders.RegisterF(int32(cmd_pb.CommandType_SyncSnapshot), network.decodeSyncSnapshotCommand)

	routerCommPool, err := network.createRouterCommPool(config)
	if err != nil {
		return nil, err
	}
	network.Inspections = NewInspectionsManager(network)
	network.RouterMessaging = NewRouterMessaging(env, routerCommPool)

	env.GetManagers().Router.Store.AddEntityIdListener(network.HandleRouterDelete, boltz.EntityDeletedAsync)

	network.AddCapability("ziti.fabric")
	network.showOptions()
	network.relayControllerMetrics()
	network.AddRouterPresenceHandler(network.RouterMessaging)
	go network.RouterMessaging.run()

	return network, nil
}

func (self *Network) HandleRouterDelete(id string) {
	self.routerDeleted(id)
	self.RouterMessaging.RouterDeleted(id)
}

func (self *Network) decodeSyncSnapshotCommand(_ int32, data []byte) (command.Command, error) {
	msg := &cmd_pb.SyncSnapshotCommand{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}

	cmd := &command.SyncSnapshotCommand{
		TimelineId:   msg.SnapshotId,
		Snapshot:     msg.Snapshot,
		SnapshotSink: self.RestoreSnapshot,
	}

	return cmd, nil
}

func routerCommunicationsWorker(_ uint32, f func()) {
	f()
}

func (network *Network) createRouterCommPool(config Config) (goroutines.Pool, error) {
	poolConfig := goroutines.PoolConfig{
		QueueSize:   config.GetOptions().RouterComm.QueueSize,
		MinWorkers:  0,
		MaxWorkers:  config.GetOptions().RouterComm.MaxWorkers,
		IdleTime:    30 * time.Second,
		CloseNotify: config.GetCloseNotify(),
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().WithField(logrus.ErrorKey, err).WithField("backtrace", string(debug.Stack())).Error("panic during message send to router")
		},
		WorkerFunction: routerCommunicationsWorker,
	}

	fabricMetrics.ConfigureGoroutinesPoolMetrics(&poolConfig, config.GetMetricsRegistry(), "pool.router.messaging")

	pool, err := goroutines.NewPool(poolConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating router messaging pool (%w)", err)
	}
	return pool, nil
}

func (network *Network) relayControllerMetrics() {
	go func() {
		timer := time.NewTicker(network.options.MetricsReportInterval)
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				if msg := network.metricsRegistry.Poll(); msg != nil {
					network.eventDispatcher.AcceptMetricsMsg(msg)
				}
			case <-network.closeNotify:
				return
			}
		}
	}()
}

func (network *Network) InitServiceCounterDispatch(handler metrics.Handler) {
	network.serviceEventMetrics.StartReporting(handler, network.GetOptions().MetricsReportInterval, 10)
}

func (network *Network) GetAppId() string {
	return network.nodeId
}

func (network *Network) GetOptions() *config.NetworkConfig {
	return network.options
}

func (network *Network) GetDb() boltz.Db {
	return network.config.GetDb()
}

func (network *Network) GetStores() *db.Stores {
	return network.env.GetStores()
}

func (network *Network) GetConnectedRouter(routerId string) *model.Router {
	return network.Router.GetConnected(routerId)
}

func (network *Network) GetReloadedRouter(routerId string) (*model.Router, error) {
	network.Router.RemoveFromCache(routerId)
	return network.Router.Read(routerId)
}

func (network *Network) GetRouter(routerId string) (*model.Router, error) {
	return network.Router.Read(routerId)
}

func (network *Network) AllConnectedRouters() []*model.Router {
	return network.Router.AllConnected()
}

func (network *Network) GetLink(linkId string) (*model.Link, bool) {
	return network.Link.Get(linkId)
}

func (network *Network) GetAllLinks() []*model.Link {
	return network.Link.All()
}

func (network *Network) GetAllLinksForRouter(routerId string) []*model.Link {
	r := network.GetConnectedRouter(routerId)
	if r == nil {
		return nil
	}
	return r.GetLinks()
}

func (network *Network) GetCircuit(circuitId string) (*model.Circuit, bool) {
	return network.Circuit.Get(circuitId)
}

func (network *Network) GetAllCircuits() []*model.Circuit {
	return network.Circuit.All()
}

func (network *Network) GetCircuitStore() *objectz.ObjectStore[*model.Circuit] {
	return network.Circuit.GetStore()
}

func (network *Network) GetLinkStore() *objectz.ObjectStore[*model.Link] {
	return network.Link.GetStore()
}

func (network *Network) RouteResult(rs *RouteStatus) bool {
	return network.routeSenderController.forwardRouteResult(rs)
}

func (network *Network) newRouteSender(circuitId string) *routeSender {
	rs := newRouteSender(circuitId, network.options.RouteTimeout, network, network.Terminator)
	network.routeSenderController.addRouteSender(rs)
	return rs
}

func (network *Network) removeRouteSender(rs *routeSender) {
	network.routeSenderController.removeRouteSender(rs)
}

func (network *Network) GetEventDispatcher() event.Dispatcher {
	return network.eventDispatcher
}

func (network *Network) GetTraceController() trace.Controller {
	return network.traceController
}

func (network *Network) GetMetricsRegistry() metrics.Registry {
	return network.metricsRegistry
}

func (network *Network) GetServiceEventsMetricsRegistry() metrics.UsageRegistry {
	return network.serviceEventMetrics
}

func (network *Network) GetCloseNotify() <-chan struct{} {
	return network.closeNotify
}

func (network *Network) ConnectedRouter(id string) bool {
	return network.Router.IsConnected(id)
}

func (network *Network) ConnectRouter(r *model.Router) {
	network.Link.BuildRouterLinks(r)
	network.Router.MarkConnected(r)

	for _, h := range network.routerPresenceHandlers.Value() {
		if syncCapableHandler, ok := h.(model.SyncRouterPresenceHandler); ok && syncCapableHandler.InvokeRouterConnectedSynchronously() {
			h.RouterConnected(r)
		} else {
			go h.RouterConnected(r)
		}
	}
	go network.ValidateTerminators(r)
}

func (network *Network) ValidateTerminators(r *model.Router) {
	logger := pfxlog.Logger().WithField("routerId", r.Id)
	result, err := network.Terminator.Query(fmt.Sprintf(`router.id = "%v" limit none`, r.Id))
	if err != nil {
		logger.WithError(err).Error("failed to get terminators for router")
		return
	}

	logger.Debugf("%v terminators to validate", len(result.Entities))
	if len(result.Entities) == 0 {
		return
	}

	network.RouterMessaging.ValidateRouterTerminators(result.Entities)
}

type LinkValidationCallback func(detail *mgmt_pb.RouterLinkDetails)

func (n *Network) ValidateLinks(filter string, cb LinkValidationCallback) (int64, func(), error) {
	result, err := n.Router.BaseList(filter)
	if err != nil {
		return 0, nil, err
	}

	sem := concurrenz.NewSemaphore(10)

	evalF := func() {
		for _, router := range result.Entities {
			connectedRouter := n.GetConnectedRouter(router.Id)
			if connectedRouter != nil {
				sem.Acquire()
				go func() {
					defer sem.Release()
					n.ValidateRouterLinks(connectedRouter, cb)
				}()
			} else {
				n.reportRouterLinksError(router, errors.New("router not connected"), cb)
			}
		}
	}

	return int64(len(result.Entities)), evalF, nil
}

type SdkTerminatorValidationCallback func(detail *mgmt_pb.RouterSdkTerminatorsDetails)

func (n *Network) ValidateRouterSdkTerminators(filter string, cb SdkTerminatorValidationCallback) (int64, func(), error) {
	result, err := n.Router.BaseList(filter)
	if err != nil {
		return 0, nil, err
	}

	sem := concurrenz.NewSemaphore(10)

	evalF := func() {
		for _, router := range result.Entities {
			connectedRouter := n.GetConnectedRouter(router.Id)
			if connectedRouter != nil {
				sem.Acquire()
				go func() {
					defer sem.Release()
					n.Router.ValidateRouterSdkTerminators(connectedRouter, cb)
				}()
			} else {
				n.Router.ReportRouterSdkTerminatorsError(router, errors.New("router not connected"), cb)
			}
		}
	}

	return int64(len(result.Entities)), evalF, nil
}

type ErtTerminatorValidationCallback func(detail *mgmt_pb.RouterErtTerminatorsDetails)

func (n *Network) ValidateRouterErtTerminators(filter string, cb ErtTerminatorValidationCallback) (int64, func(), error) {
	result, err := n.Router.BaseList(filter)
	if err != nil {
		return 0, nil, err
	}

	sem := concurrenz.NewSemaphore(10)

	evalF := func() {
		for _, router := range result.Entities {
			connectedRouter := n.GetConnectedRouter(router.Id)
			if connectedRouter != nil {
				sem.Acquire()
				go func() {
					defer sem.Release()
					n.Router.ValidateRouterErtTerminators(connectedRouter, cb)
				}()
			} else {
				n.Router.ReportRouterErtTerminatorsError(router, errors.New("router not connected"), cb)
			}
		}
	}

	return int64(len(result.Entities)), evalF, nil
}

func (network *Network) DisconnectRouter(r *model.Router) {
	// 1: remove Links for Router
	for _, l := range r.GetLinks() {
		wasConnected := l.CurrentState().Mode == model.Connected
		if l.Src.Id == r.Id {
			network.Link.Remove(l)
		}
		if wasConnected {
			network.RerouteLink(l)
		}
	}
	// 2: remove Router
	network.Router.MarkDisconnected(r)

	for _, h := range network.routerPresenceHandlers.Value() {
		h.RouterDisconnected(r)
	}
}

func (network *Network) NotifyExistingLink(srcRouter *model.Router, reportedLink *ctrl_pb.RouterLinks_RouterLink) {
	log := pfxlog.Logger().
		WithField("routerId", srcRouter.Id).
		WithField("linkId", reportedLink.Id).
		WithField("destRouterId", reportedLink.DestRouterId).
		WithField("iteration", reportedLink.Iteration)

	src := network.Router.GetConnected(srcRouter.Id)
	if src == nil {
		log.Info("ignoring links message processed after router disconnected")
		return
	}

	if src != srcRouter || !srcRouter.Connected.Load() {
		log.Info("ignoring links message processed from old router connection")
		return
	}

	dst := network.Router.GetConnected(reportedLink.DestRouterId)
	if dst == nil {
		network.NotifyLinkIdEvent(reportedLink.Id, event.LinkFromRouterDisconnectedDest)
	}

	link, created := network.Link.RouterReportedLink(reportedLink, src, dst)
	if created {
		network.NotifyLinkEvent(link, event.LinkFromRouterNew)
		log.Info("router reported link added")
	} else {
		network.NotifyLinkEvent(link, event.LinkFromRouterKnown)
		log.Info("router reported link already known")
	}
}

func (network *Network) LinkFaulted(link *model.Link, dupe bool) {
	wasUsable := link.IsUsable()

	link.SetState(model.Failed)
	if dupe {
		network.NotifyLinkEvent(link, event.LinkDuplicate)
	} else {
		network.NotifyLinkEvent(link, event.LinkFault)
	}

	pfxlog.Logger().WithField("linkId", link.Id).Info("removing failed link")
	network.Link.Remove(link)

	if wasUsable {
		network.RerouteLink(link)
	}
}

func (network *Network) VerifyRouter(routerId string, fingerprints []string) error {
	router, err := network.GetRouter(routerId)
	if err != nil {
		return err
	}

	routerFingerprint := router.Fingerprint
	if routerFingerprint == nil {
		return fmt.Errorf("invalid router %v, not yet enrolled", routerId)
	}

	for _, fp := range fingerprints {
		if fp == *routerFingerprint {
			return nil
		}
	}

	return fmt.Errorf("could not verify fingerprint for router %v", routerId)
}

func (network *Network) RerouteLink(l *model.Link) {
	// This is called from Channel.rxer() and thus may not block
	go func() {
		network.handleRerouteLink(l)
	}()
}

func (network *Network) CreateCircuit(params model.CreateCircuitParams) (*model.Circuit, error) {
	clientId := params.GetClientId()
	service := params.GetServiceId()
	ctx := params.GetLogContext()
	deadline := params.GetDeadline()

	startTime := time.Now()

	instanceId, serviceId := parseInstanceIdAndService(service)

	// 1: Allocate Circuit Identifier
	circuitId, err := idgen.NewUUIDString()
	if err != nil {
		network.CircuitFailedEvent(circuitId, params, startTime, nil, nil, CircuitFailureIdGenerationError)
		return nil, err
	}
	ctx.WithFields(map[string]interface{}{
		"circuitId":     circuitId,
		"serviceId":     service,
		"attemptNumber": 1,
	})
	logger := pfxlog.ChannelLogger(logcontext.SelectPath).Wire(ctx).Entry

	circuit := &model.Circuit{
		Id:        circuitId,
		ClientId:  clientId.Token,
		ServiceId: serviceId,
	}

	attempt := uint32(0)
	allCleanups := make(map[string]struct{})
	rs := network.newRouteSender(circuitId)
	defer func() { network.removeRouteSender(rs) }()
	for {
		// 2: Find Service
		svc, err := network.Service.Read(serviceId)
		if err != nil {
			network.CircuitFailedEvent(circuitId, params, startTime, nil, nil, CircuitFailureInvalidService)
			network.ServiceDialOtherError(serviceId)
			return circuit, err
		}
		logger = logger.WithField("serviceName", svc.Name)

		// 3: select terminator
		strategy, terminator, pathNodes, strategyData, circuitErr := network.selectPath(params, svc, instanceId, ctx)
		if circuitErr != nil {
			network.CircuitFailedEvent(circuitId, params, startTime, nil, nil, circuitErr.Cause())
			network.ServiceDialOtherError(serviceId)
			return circuit, circuitErr
		}

		circuit.Terminator = terminator

		// 4: Create Path
		path, pathErr := network.CreatePathWithNodes(pathNodes)
		if pathErr != nil {
			network.CircuitFailedEvent(circuitId, params, startTime, nil, terminator, pathErr.Cause())
			network.ServiceDialOtherError(serviceId)
			return circuit, pathErr
		}

		circuit.Path = path

		// get circuit tags
		tags := params.GetCircuitTags(terminator)

		circuit.Tags = tags

		// 4a: Create Route Messages
		rms := network.CreateRouteMessages(path, attempt, circuitId, terminator, deadline)
		rms[len(rms)-1].Egress.PeerData = clientId.Data
		for _, msg := range rms {
			msg.Context = &ctrl_pb.Context{
				Fields:      ctx.GetStringFields(),
				ChannelMask: ctx.GetChannelsMask(),
			}
			msg.Tags = tags
		}

		// 5: Routing
		logger.Debug("route attempt for circuit")
		peerData, cleanups, circuitErr := rs.route(attempt, path, rms, strategy, terminator, ctx.Clone())
		for k, v := range cleanups {
			allCleanups[k] = v
		}
		if circuitErr != nil {
			logger.WithError(circuitErr).Warn("route attempt for circuit failed")
			network.CircuitFailedEvent(circuitId, params, startTime, path, terminator, circuitErr.Cause())
			attempt++
			ctx.WithField("attemptNumber", attempt)
			logger = logger.WithField("attemptNumber", attempt)
			if attempt < network.options.CreateCircuitRetries {
				continue
			} else {
				// revert successful routes
				logger.Warnf("circuit creation failed after [%d] attempts, sending cleanup unroutes", network.options.CreateCircuitRetries)
				for cleanupRId := range allCleanups {
					if r := network.GetConnectedRouter(cleanupRId); r != nil {
						if err := sendUnroute(r, circuitId, true); err == nil {
							logger.WithField("routerId", cleanupRId).Debug("sent cleanup unroute for circuit")
						} else {
							logger.WithField("routerId", cleanupRId).Error("error sending cleanup unroute for circuit")
						}
					} else {
						logger.WithField("routerId", cleanupRId).Error("router for circuit cleanup not connected")
					}
				}

				return circuit, fmt.Errorf("exceeded maximum [%d] retries creating circuit [c/%s] (%w)", network.options.CreateCircuitRetries, circuitId, circuitErr)
			}
		}

		// 5.a: Unroute Abandoned Routers (from Previous Attempts)
		usedRouters := make(map[string]struct{})
		for _, r := range path.Nodes {
			usedRouters[r.Id] = struct{}{}
		}
		cleanupCount := 0
		for cleanupRId := range allCleanups {
			if _, found := usedRouters[cleanupRId]; !found {
				cleanupCount++
				if r := network.GetConnectedRouter(cleanupRId); r != nil {
					if err := sendUnroute(r, circuitId, true); err == nil {
						logger.WithField("routerId", cleanupRId).Debug("sent abandoned cleanup unroute for circuit to router")
					} else {
						logger.WithField("routerId", cleanupRId).WithError(err).Error("error sending abandoned cleanup unroute for circuit to router")
					}
				} else {
					logger.WithField("routerId", cleanupRId).Error("router not connected for circuit, abandoned cleanup")
				}
			}
		}
		logger.Debugf("cleaned up [%d] abandoned routers for circuit", cleanupCount)

		path.InitiatorLocalAddr = string(clientId.Data[uint32(ctrl_msg.InitiatorLocalAddressHeader)])
		path.InitiatorRemoteAddr = string(clientId.Data[uint32(ctrl_msg.InitiatorRemoteAddressHeader)])
		path.TerminatorLocalAddr = string(peerData[uint32(ctrl_msg.TerminatorLocalAddressHeader)])
		path.TerminatorRemoteAddr = string(peerData[uint32(ctrl_msg.TerminatorRemoteAddressHeader)])

		delete(peerData, uint32(ctrl_msg.InitiatorLocalAddressHeader))
		delete(peerData, uint32(ctrl_msg.InitiatorRemoteAddressHeader))
		delete(peerData, uint32(ctrl_msg.TerminatorLocalAddressHeader))
		delete(peerData, uint32(ctrl_msg.TerminatorRemoteAddressHeader))

		for k, v := range strategyData {
			peerData[k] = v
		}

		now := time.Now()
		// 6: Create Circuit Object
		circuit.PeerData = peerData
		circuit.CreatedAt = now
		circuit.UpdatedAt = now

		network.Circuit.Add(circuit)
		creationTimespan := time.Since(startTime)
		network.CircuitEvent(event.CircuitCreated, circuit, &creationTimespan)

		logger.WithField("path", circuit.Path).
			WithField("terminator_local_address", circuit.Path.TerminatorLocalAddr).
			WithField("terminator_remote_address", circuit.Path.TerminatorRemoteAddr).
			Debug("created circuit")
		return circuit, nil
	}
}

func (network *Network) ReportForwardingFaults(ffr *ForwardingFaultReport) {
	go network.handleForwardingFaults(ffr)
}

func parseInstanceIdAndService(service string) (string, string) {
	atIndex := strings.IndexRune(service, '@')
	if atIndex < 0 {
		return "", service
	}
	identityId := service[0:atIndex]
	serviceId := service[atIndex+1:]
	return identityId, serviceId
}

// matchesTerminatorInstanceId checks if a requested instanceId matches a terminator's
// instanceId pattern. Supports exact matching and wildcard subdomain matching (prefix "*:").
// Non-wildcard terminators (including empty InstanceId) use exact case-insensitive matching,
// preserving backward compatibility where empty only matches empty.
func matchesTerminatorInstanceId(terminatorInstanceId, requestedInstanceId string) bool {
	if !strings.HasPrefix(terminatorInstanceId, "*:") {
		return strings.EqualFold(terminatorInstanceId, requestedInstanceId)
	}

	if requestedInstanceId == "" {
		return false
	}

	basePattern := strings.ToLower(terminatorInstanceId[2:])
	requested := strings.ToLower(requestedInstanceId)

	if requested == basePattern {
		return true
	}

	return strings.HasSuffix(requested, "."+basePattern)
}

// wildcardBaseLen returns the length of the base pattern in a terminator's InstanceId
// for specificity ranking. Longer base = more specific match.
//   - "" → 0 (catch-all, least specific)
//   - "node1.ziti.internal" (exact) → math.MaxInt (most specific)
//   - "*:node1.ziti.internal" → 19
//   - "*:ziti.internal" → 13
func wildcardBaseLen(terminatorInstanceId string) int {
	if terminatorInstanceId == "" {
		return 0
	}
	if !strings.HasPrefix(terminatorInstanceId, "*:") {
		return math.MaxInt
	}
	return len(terminatorInstanceId) - 2
}

func (network *Network) selectPath(params model.CreateCircuitParams, svc *model.Service, instanceId string, ctx logcontext.Context) (xt.Strategy, xt.CostedTerminator, []*model.Router, xt.PeerData, CircuitError) {
	paths := map[string]*PathAndCost{}
	var weightedTerminators []xt.CostedTerminator
	var errList []error

	log := pfxlog.ChannelLogger(logcontext.SelectPath).Wire(ctx)

	hasOfflineRouters := false
	pathError := false
	bestSpecificity := -1

	for _, terminator := range svc.Terminators {
		if !matchesTerminatorInstanceId(terminator.InstanceId, instanceId) {
			continue
		}

		specificity := wildcardBaseLen(terminator.InstanceId)

		// Skip if less specific than what we already have
		if specificity < bestSpecificity {
			continue
		}

		pathAndCost, found := paths[terminator.Router]
		if !found {
			dstR := network.Router.GetConnected(terminator.GetRouterId())
			if dstR == nil {
				err := fmt.Errorf("router with id=%v on terminator with id=%v for service name=%v is not online",
					terminator.GetRouterId(), terminator.GetId(), svc.Name)
				log.Debugf("error while calculating path for service %v: %v", svc.Id, err)

				errList = append(errList, err)
				hasOfflineRouters = true
				continue
			}

			path, cost, err := network.shortestPath(params.GetSourceRouter(), dstR)
			if err != nil {
				log.Debugf("error while calculating path for service %v: %v", svc.Id, err)
				errList = append(errList, err)
				pathError = true
				continue
			}

			pathAndCost = newPathAndCost(path, cost)
			paths[terminator.GetRouterId()] = pathAndCost
		}

		// If more specific than previous candidates, discard them
		if specificity > bestSpecificity {
			weightedTerminators = weightedTerminators[:0]
			bestSpecificity = specificity
		}

		dynamicCost := xt.GlobalCosts().GetDynamicCost(terminator.Id)
		unbiasedCost := uint32(terminator.Cost) + uint32(dynamicCost) + pathAndCost.cost
		biasedCost := terminator.Precedence.GetBiasedCost(unbiasedCost)
		costedTerminator := &model.RoutingTerminator{
			Terminator: terminator,
			RouteCost:  biasedCost,
		}
		weightedTerminators = append(weightedTerminators, costedTerminator)
	}

	if bestSpecificity >= 0 && bestSpecificity < math.MaxInt {
		log.Debugf("using wildcard terminator match (specificity=%d) for service %v, instanceId %v",
			bestSpecificity, svc.Id, instanceId)
	}

	if len(svc.Terminators) == 0 {
		return nil, nil, nil, nil, newCircuitErrorf(CircuitFailureNoTerminators, "service %v has no terminators", svc.Id)
	}

	if len(weightedTerminators) == 0 {
		if pathError {
			return nil, nil, nil, nil, newCircuitErrWrap(CircuitFailureNoPath, errors.Join(errList...))
		}

		if hasOfflineRouters {
			return nil, nil, nil, nil, newCircuitErrorf(CircuitFailureNoOnlineTerminators, "service %v has no online terminators for instanceId %v", svc.Id, instanceId)
		}

		return nil, nil, nil, nil, newCircuitErrorf(CircuitFailureNoTerminators, "service %v has no terminators for instanceId %v", svc.Id, instanceId)
	}

	strategy, err := network.strategyRegistry.GetStrategy(svc.TerminatorStrategy)
	if err != nil {
		return nil, nil, nil, nil, newCircuitErrWrap(CircuitFailureInvalidStrategy, err)
	}

	sort.Slice(weightedTerminators, func(i, j int) bool {
		return weightedTerminators[i].GetRouteCost() < weightedTerminators[j].GetRouteCost()
	})

	terminator, peerData, err := strategy.Select(params, weightedTerminators)

	if err != nil {
		return nil, nil, nil, nil, newCircuitErrorf(CircuitFailureStrategyError, "strategy %v errored selecting terminator for service %v: %v", svc.TerminatorStrategy, svc.Id, err)
	}

	if terminator == nil {
		return nil, nil, nil, nil, newCircuitErrorf(CircuitFailureStrategyError, "strategy %v did not select terminator for service %v", svc.TerminatorStrategy, svc.Id)
	}

	path := paths[terminator.GetRouterId()].path

	if log.Logger.IsLevelEnabled(logrus.DebugLevel) {
		buf := strings.Builder{}
		buf.WriteString("[")
		if len(weightedTerminators) > 0 {
			buf.WriteString(fmt.Sprintf("%v: %v", weightedTerminators[0].GetId(), weightedTerminators[0].GetRouteCost()))
			for _, t := range weightedTerminators[1:] {
				buf.WriteString(", ")
				buf.WriteString(fmt.Sprintf("%v: %v", t.GetId(), t.GetRouteCost()))
			}
		}
		buf.WriteString("]")
		var routerIds []string
		for _, r := range path {
			routerIds = append(routerIds, fmt.Sprintf("r/%s", r.Id))
		}
		pathStr := strings.Join(routerIds, "->")
		log.Debugf("selected terminator %v for path %v from %v", terminator.GetId(), pathStr, buf.String())
	}

	return strategy, terminator, path, peerData, nil
}

func (network *Network) RemoveCircuit(circuitId string, now bool) error {
	log := pfxlog.Logger().WithField("circuitId", circuitId)

	if circuit, found := network.Circuit.Get(circuitId); found {
		for _, r := range circuit.Path.Nodes {
			err := sendUnroute(r, circuit.Id, now)
			if err != nil {
				log.Errorf("error sending unroute to [r/%s] (%s)", r.Id, err)
			}
		}

		network.Circuit.Remove(circuit)
		network.CircuitEvent(event.CircuitDeleted, circuit, nil)

		if svc, err := network.Service.Read(circuit.ServiceId); err == nil {
			if strategy, err := network.strategyRegistry.GetStrategy(svc.TerminatorStrategy); strategy != nil {
				strategy.NotifyEvent(xt.NewCircuitRemoved(circuit.Terminator))
			} else if err != nil {
				log.WithError(err).WithField("terminatorStrategy", svc.TerminatorStrategy).Warn("failed to notify strategy of circuit end, invalid strategy")
			}
		} else {
			log.WithError(err).Error("unable to get service for circuit")
		}

		log.Debug("removed circuit")

		return nil
	}
	return InvalidCircuitError{circuitId: circuitId}
}

func (network *Network) CreatePath(srcR, dstR *model.Router) (*model.Path, error) {
	ingressId, err := idgen.NewUUIDString()
	if err != nil {
		return nil, err
	}

	egressId, err := idgen.NewUUIDString()
	if err != nil {
		return nil, err
	}

	path := &model.Path{
		Links:     make([]*model.Link, 0),
		IngressId: ingressId,
		EgressId:  egressId,
		Nodes:     make([]*model.Router, 0),
	}
	path.Nodes = append(path.Nodes, srcR)
	path.Nodes = append(path.Nodes, dstR)

	return network.UpdatePath(path)
}

func (network *Network) setLinks(path *model.Path) error {
	if len(path.Nodes) > 1 {
		for i := 0; i < len(path.Nodes)-1; i++ {
			if link, found := network.Link.LeastExpensiveLink(path.Nodes[i], path.Nodes[i+1]); found {
				path.Links = append(path.Links, link)
			} else {
				return fmt.Errorf("no link from r/%v to r/%v", path.Nodes[i].Id, path.Nodes[i+1].Id)
			}
		}
	}
	return nil
}

func (network *Network) AddRouterPresenceHandler(h model.RouterPresenceHandler) {
	network.routerPresenceHandlers.Append(h)
}

func (network *Network) Run() {
	defer logrus.Info("exited")
	logrus.Info("started")

	go network.watchdog()

	ticker := time.NewTicker(time.Duration(network.options.CycleSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			network.clean()
			network.smart()
			network.Link.ScanForDeadLinks()

		case <-network.closeNotify:
			network.eventDispatcher.RemoveMetricsMessageHandler(network)
			network.metricsRegistry.DisposeAll()
			return
		}

		// notify the watchdog that we're processing
		select {
		case network.watchdogCh <- struct{}{}:
		default:
		}
	}
}

func (network *Network) watchdog() {
	watchdogInterval := 2 * time.Duration(network.options.CycleSeconds) * time.Second
	consecutiveFails := 0

	watchDogTicker := time.NewTicker(watchdogInterval)
	defer watchDogTicker.Stop()

	for {
		// check every 2x cycle seconds
		select {
		case <-watchDogTicker.C:
		case <-network.closeNotify:
			return
		}

		select {
		case <-network.watchdogCh:
			consecutiveFails = 0
			continue
		case <-network.closeNotify:
			return
		default:
			consecutiveFails++
			// network.Run didn't complete, something is stalling it
			pfxlog.Logger().
				WithField("watchdogInterval", watchdogInterval.String()).
				WithField("consecutiveFails", consecutiveFails).
				Warn("network.Run did not finish within watchdog interval")

			if consecutiveFails == 3 {
				debugz.DumpStack()
			}
		}
	}
}

func (network *Network) handleRerouteLink(l *model.Link) {
	log := logrus.WithField("linkId", l.Id)
	log.Info("changed link")
	if err := network.rerouteLink(l, time.Now().Add(config.DefaultOptionsRouteTimeout)); err != nil {
		log.WithError(err).Error("unexpected error rerouting link")
	}
}

func (network *Network) handleForwardingFaults(ffr *ForwardingFaultReport) {
	network.fault(ffr)
}

func (network *Network) AddCapability(capability string) {
	network.lock.Lock()
	defer network.lock.Unlock()
	network.capabilities = append(network.capabilities, capability)
}

func (network *Network) GetCapabilities() []string {
	network.lock.Lock()
	defer network.lock.Unlock()
	return network.capabilities
}

func (network *Network) RemoveLink(linkId string) {
	log := pfxlog.Logger().WithField("linkId", linkId)

	link, _ := network.Link.Get(linkId)
	var iteration uint32

	var routerList []*model.Router
	if link != nil {
		iteration = link.Iteration
		routerList = []*model.Router{link.Src}
		if dst := link.GetDest(); dst != nil {
			routerList = append(routerList, dst)
		}
		log = log.WithField("srcRouterId", link.Src.Id).
			WithField("dstRouterId", link.DstId).
			WithField("iteration", iteration)
		log.Info("deleting known link")
	} else {
		routerList = network.AllConnectedRouters()
		log.Info("deleting unknown link (sending link fault to all connected routers)")
	}

	for _, router := range routerList {
		fault := &ctrl_pb.Fault{
			Subject:   ctrl_pb.FaultSubject_LinkFault,
			Id:        linkId,
			Iteration: iteration,
		}

		if ctrl := router.Control; ctrl != nil {
			if err := protobufs.MarshalTyped(fault).WithTimeout(15 * time.Second).Send(ctrl.GetDefaultSender()); err != nil {
				log.WithField("faultDestRouterId", router.Id).WithError(err).
					Error("failed to send link fault to router on link removal")
			} else {
				log.WithField("faultDestRouterId", router.Id).WithError(err).
					Info("sent link fault to router on link removal")
			}
		}
	}

	if link != nil {
		network.Link.Remove(link)
		network.RerouteLink(link)
	}
}

func (network *Network) rerouteLink(l *model.Link, deadline time.Time) error {
	circuits := network.Circuit.All()
	for _, circuit := range circuits {
		if circuit.Path.UsesLink(l) {
			log := logrus.WithField("linkId", l.Id).
				WithField("circuitId", circuit.Id)
			log.Info("circuit uses link")
			if err := network.rerouteCircuit(circuit, deadline); err != nil {
				log.WithError(err).Error("error rerouting circuit, removing")
				if err := network.RemoveCircuit(circuit.Id, true); err != nil {
					log.WithError(err).Error("error removing circuit after reroute failure")
				}
			}
		}
	}

	return nil
}

func (network *Network) rerouteCircuitWithTries(circuit *model.Circuit, retries int) bool {
	log := pfxlog.Logger().WithField("circuitId", circuit.Id)

	for i := 0; i < retries; i++ {
		deadline := time.Now().Add(config.DefaultOptionsRouteTimeout)
		err := network.rerouteCircuit(circuit, deadline)
		if err == nil {
			return true
		}

		log.WithError(err).WithField("attempt", i).Error("error re-routing circuit")
	}

	if err := network.RemoveCircuit(circuit.Id, true); err != nil {
		log.WithError(err).Error("failure while removing circuit after failed re-route attempt")
	}
	return false
}

func (network *Network) rerouteCircuit(circuit *model.Circuit, deadline time.Time) error {
	log := pfxlog.Logger().WithField("circuitId", circuit.Id)
	if circuit.Rerouting.CompareAndSwap(false, true) {
		defer circuit.Rerouting.Store(false)

		log.Warn("rerouting circuit")

		if cq, err := network.UpdatePath(circuit.Path); err == nil {
			circuit.Path = cq
			circuit.UpdatedAt = time.Now()

			rms := network.CreateRouteMessages(cq, SmartRerouteAttempt, circuit.Id, circuit.Terminator, deadline)

			for i := 0; i < len(cq.Nodes); i++ {
				if _, err := sendRoute(cq.Nodes[i], rms[i], network.options.RouteTimeout); err != nil {
					log.WithError(err).Errorf("error sending route to [r/%s]", cq.Nodes[i].Id)
				}
			}

			log.Info("rerouted circuit")

			network.CircuitEvent(event.CircuitUpdated, circuit, nil)
			return nil
		} else {
			return err
		}
	} else {
		log.Info("not rerouting circuit, already in progress")
		return nil
	}
}

func (network *Network) smartReroute(circuit *model.Circuit, cq *model.Path, deadline time.Time) bool {
	retry := false
	log := pfxlog.Logger().WithField("circuitId", circuit.Id)
	if circuit.Rerouting.CompareAndSwap(false, true) {
		defer circuit.Rerouting.Store(false)

		circuit.Path = cq
		circuit.UpdatedAt = time.Now()

		rms := network.CreateRouteMessages(cq, SmartRerouteAttempt, circuit.Id, circuit.Terminator, deadline)

		for i := 0; i < len(cq.Nodes); i++ {
			if _, err := sendRoute(cq.Nodes[i], rms[i], network.options.RouteTimeout); err != nil {
				retry = true
				log.WithField("routerId", cq.Nodes[i].Id).WithError(err).Error("error sending smart route update to router")
				break
			}
		}

		if !retry {
			logrus.Debug("rerouted circuit")
			network.CircuitEvent(event.CircuitUpdated, circuit, nil)
		}
	}
	return retry
}

func (network *Network) AcceptMetricsMsg(metrics *metrics_pb.MetricsMessage) {
	if metrics.SourceId == network.nodeId {
		return // ignore metrics coming from the controller itself
	}

	log := pfxlog.Logger()

	router, err := network.Router.Read(metrics.SourceId)
	if err != nil {
		log.Debugf("could not find router [r/%s] while processing metrics", metrics.SourceId)
		return
	}

	for _, link := range network.GetAllLinksForRouter(router.Id) {
		metricId := "link." + link.Id + ".latency"
		var latencyCost int64
		var found bool
		if latency, ok := metrics.Histograms[metricId]; ok {
			latencyCost = int64(latency.Mean)
			found = true

			metricId = "link." + link.Id + ".queue_time"
			if queueTime, ok := metrics.Histograms[metricId]; ok {
				latencyCost += int64(queueTime.Mean)
			}
		}

		if found {
			if link.Src.Id == router.Id {
				link.SetSrcLatency(latencyCost) // latency is in nanoseconds
			} else if link.DstId == router.Id {
				link.SetDstLatency(latencyCost) // latency is in nanoseconds
			} else {
				log.Warnf("link not for router")
			}
		}
	}
}

func sendRoute(r *model.Router, createMsg *ctrl_pb.Route, timeout time.Duration) (xt.PeerData, error) {
	log := pfxlog.Logger().WithField("routerId", r.Id).
		WithField("circuitId", createMsg.CircuitId)

	log.Debug("sending create route message")

	msg, err := protobufs.MarshalTyped(createMsg).WithTimeout(timeout).SendForReply(r.Control.GetHighPrioritySender())
	if err != nil {
		log.WithError(err).WithField("timeout", timeout).Error("error sending route message")
		return nil, err
	}

	if msg.ContentType == ctrl_msg.RouteResultType {
		_, success := msg.Headers[ctrl_msg.RouteResultSuccessHeader]
		if !success {
			message := "route error, but no error message from router"
			if errMsg, found := msg.Headers[ctrl_msg.RouteResultErrorHeader]; found {
				message = string(errMsg)
			}
			return nil, errors.New(message)
		}

		peerData := xt.PeerData{}
		for k, v := range msg.Headers {
			if k > 0 {
				peerData[uint32(k)] = v
			}
		}

		return peerData, nil
	}
	return nil, fmt.Errorf("unexpected response type %v received in reply to route request", msg.ContentType)
}

func sendUnroute(r *model.Router, circuitId string, now bool) error {
	unroute := &ctrl_pb.Unroute{
		CircuitId: circuitId,
		Now:       now,
	}
	return protobufs.MarshalTyped(unroute).Send(r.Control.GetHighPrioritySender())
}

func (network *Network) showOptions() {
	if jsonOptions, err := json.MarshalIndent(network.options, "", "  "); err == nil {
		pfxlog.Logger().Infof("network = %s", string(jsonOptions))
	} else {
		panic(err)
	}
}

type renderConfig interface {
	RenderJsonConfig() (string, error)
}

func (network *Network) routerDeleted(routerId string) {
	circuits := network.GetAllCircuits()
	for _, circuit := range circuits {
		if circuit.HasRouter(routerId) {
			path := circuit.Path

			// If we're either the initiator, terminator (or both), cleanup the circuit since
			// we won't be able to re-establish it, and we'll never get a circuit fault
			if path.Nodes[0].Id == routerId || path.Nodes[len(path.Nodes)-1].Id == routerId {
				if err := network.RemoveCircuit(circuit.Id, true); err != nil {
					pfxlog.Logger().WithField("routerId", routerId).
						WithField("circuitId", circuit.Id).
						WithError(err).Error("unable to remove circuit after router was deleted")
				}
			}
		}
	}
}

var DbSnapshotTooFrequentError = dbSnapshotTooFrequentError{}

type dbSnapshotTooFrequentError struct{}

func (d dbSnapshotTooFrequentError) Error() string {
	return "may snapshot database at most once per minute"
}

func (network *Network) SnapshotDatabase() error {
	_, err := network.SnapshotDatabaseToFile("")
	return err
}

func (network *Network) SnapshotDatabaseToFile(path string) (string, error) {
	network.lock.Lock()
	defer network.lock.Unlock()

	if network.lastSnapshot.Add(time.Minute).After(time.Now()) {
		return "", DbSnapshotTooFrequentError
	}

	actualPath := path
	if actualPath == "" {
		actualPath = "__DB_DIR__/__DB_FILE__-__DATE__-__TIME__"
	}

	err := network.GetDb().View(func(tx *bbolt.Tx) error {
		currentIndex := db.LoadCurrentRaftIndex(tx)
		actualPath = strings.ReplaceAll(actualPath, "__RAFT_INDEX__", fmt.Sprintf("%v", currentIndex))
		actualPath = strings.ReplaceAll(actualPath, "RAFT_INDEX", fmt.Sprintf("%v", currentIndex))

		var err error
		actualPath, _, err = network.GetDb().SnapshotInTx(tx, actualPath)
		return err
	})

	if err == nil {
		network.lastSnapshot = time.Now()
	}

	return actualPath, err
}

func (network *Network) RestoreSnapshot(cmd *command.SyncSnapshotCommand, index uint64) error {
	log := pfxlog.Logger()
	currentTimelineId, err := network.GetDb().GetTimelineId(boltz.TimelineModeDefault, shortid.Generate)
	if err != nil {
		log.WithError(err).Error("unable to get current timeline id")
	}
	if currentTimelineId != "" && currentTimelineId == cmd.TimelineId {
		log.WithField("timelineId", cmd.TimelineId).Info("snapshot already current, skipping reload")
		return nil
	}

	if err != nil {
		log.WithError(err).Error("unable to read current raft index before DB restore")
	}

	buf := bytes.NewBuffer(cmd.Snapshot)
	reader, err := gzip.NewReader(buf)
	if err != nil {
		return fmt.Errorf("unable to create gz reader for reading migration snapshot during restore (%w)", err)
	}

	network.GetDb().RestoreFromReader(reader)
	err = network.GetDb().Update(nil, func(ctx boltz.MutateContext) error {
		raftBucket := boltz.GetOrCreatePath(ctx.Tx(), db.RootBucket, db.MetadataBucket)
		raftBucket.SetInt64(db.FieldRaftIndex, int64(index), nil)
		return nil
	})

	if err != nil {
		log.WithError(err).Errorf("failed to set index after restore")
	}

	time.AfterFunc(5*time.Second, func() {
		log.Info("database restore requires controller restart. exiting...")
		os.Exit(0)
	})

	return nil
}

func (network *Network) AddInspectTarget(target InspectTarget) {
	network.inspectionTargets.Append(target)
}

func (network *Network) ValidateRouterLinks(router *model.Router, cb LinkValidationCallback) {
	request := &ctrl_pb.InspectRequest{RequestedValues: []string{"links"}}
	resp := &ctrl_pb.InspectResponse{}
	respMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Minute).SendForReply(router.Control.GetDefaultSender())
	if err = protobufs.TypedResponse(resp).Unmarshall(respMsg, err); err != nil {
		network.reportRouterLinksError(router, err, cb)
		return
	}

	var linkDetails *inspect.LinksInspectResult
	for _, val := range resp.Values {
		if val.Name == "links" {
			if err = json.Unmarshal([]byte(val.Value), &linkDetails); err != nil {
				network.reportRouterLinksError(router, err, cb)
				return
			}
		}
	}

	if linkDetails == nil {
		if len(resp.Errors) > 0 {
			err = errors.New(strings.Join(resp.Errors, ","))
			network.reportRouterLinksError(router, err, cb)
			return
		}
		network.reportRouterLinksError(router, errors.New("no link details returned from router"), cb)
		return
	}

	linkMap := network.Link.GetLinkMap()

	result := &mgmt_pb.RouterLinkDetails{
		RouterId:        router.Id,
		RouterName:      router.Name,
		ValidateSuccess: true,
	}

	for _, link := range linkDetails.Links {
		detail := &mgmt_pb.RouterLinkDetail{
			LinkId:       link.Id,
			RouterState:  mgmt_pb.LinkState_LinkEstablished,
			DestRouterId: link.Dest,
			Dialed:       link.Dialed,
		}
		detail.DestConnected = network.ConnectedRouter(link.Dest)
		if ctrlLink, found := linkMap[link.Id]; found {
			detail.CtrlState = mgmt_pb.LinkState_LinkEstablished
			detail.IsValid = detail.DestConnected
			if link.Dialed { // only compare against dialer side of the link, as src/dst will be flipped on the listener side
				network.checkLinkConns(ctrlLink, link, detail)
			}
		} else {
			detail.CtrlState = mgmt_pb.LinkState_LinkUnknown
			detail.IsValid = !detail.DestConnected
		}
		delete(linkMap, link.Id)
		result.LinkDetails = append(result.LinkDetails, detail)
	}

	for _, link := range linkMap {
		related := false
		dest := ""
		if link.Src.Id == router.Id {
			related = true
			dest = link.DstId
		} else if link.DstId == router.Id {
			related = true
			dest = link.Src.Id
		}

		if related {
			detail := &mgmt_pb.RouterLinkDetail{
				LinkId:        link.Id,
				CtrlState:     mgmt_pb.LinkState_LinkEstablished,
				DestConnected: network.ConnectedRouter(dest),
				RouterState:   mgmt_pb.LinkState_LinkUnknown,
				IsValid:       false,
				DestRouterId:  dest,
				Dialed:        link.Src.Id == router.Id,
			}
			result.LinkDetails = append(result.LinkDetails, detail)
		}
	}

	cb(result)
}

func (network *Network) checkLinkConns(ctrlLink *model.Link, routerLink *inspect.LinkInspectDetail, result *mgmt_pb.RouterLinkDetail) {
	sortF := func(v []*ctrl_pb.LinkConn) []*ctrl_pb.LinkConn {
		return slices.SortedFunc(slices.Values(v), func(e *ctrl_pb.LinkConn, e2 *ctrl_pb.LinkConn) int {
			if diff := strings.Compare(e.Type, e2.Type); diff != 0 {
				return diff
			}
			if diff := strings.Compare(e.LocalAddr, e2.LocalAddr); diff != 0 {
				return diff
			}
			return strings.Compare(e.RemoteAddr, e2.RemoteAddr)
		})
	}

	var routerConns []*ctrl_pb.LinkConn

	for _, v := range routerLink.Connections {
		routerConns = append(routerConns, &ctrl_pb.LinkConn{
			Type:       v.Type,
			LocalAddr:  v.Source,
			RemoteAddr: v.Dest,
		})
	}

	// ensure that conn info is being reported
	if srcR := ctrlLink.Src; srcR != nil {
		hasMinVersion, err := srcR.VersionInfo.HasMinimumVersion("v1.6.6")
		if err != nil {
			result.IsValid = false
			result.Messages = append(result.Messages, err.Error())
			return
		}
		if !hasMinVersion {
			return
		}
	}

	ctrlConnState := ctrlLink.GetConnsState()
	ctrlConns := sortF(ctrlConnState.GetConns())
	routerConns = sortF(routerConns)

	if len(ctrlConns) != len(routerConns) {
		result.Messages = append(result.Messages, fmt.Sprintf("for link %s, len(ctrlConns): %d != len(routerConns): %d",
			ctrlLink.Id, len(ctrlConns), len(ctrlConns)))
		return
	}

	if routerLink.ConnStateIteration != ctrlConnState.GetStateIteration() {
		result.Messages = append(result.Messages, fmt.Sprintf("for link %s, conn state iteration doesn't match, ctrl: %d != router: %d",
			ctrlLink.Id, ctrlConnState.GetStateIteration(), routerLink.ConnStateIteration))
		return
	}

	for i := 0; i < len(ctrlConns); i++ {
		ctrlConn := ctrlConns[i]
		routerConn := routerConns[i]
		if ctrlConn.Type != routerConn.Type {
			result.IsValid = false
			result.Messages = append(result.Messages, fmt.Sprintf("for link %s, type doesn't match. ctrl %s != router %s",
				ctrlLink.Id, ctrlConn.Type, routerConn.Type))
		}
		if ctrlConn.LocalAddr != routerConn.LocalAddr {
			result.IsValid = false
			result.Messages = append(result.Messages, fmt.Sprintf("for link %s, local addr doesn't match. ctrl %s != router %s",
				ctrlLink.Id, ctrlConn.LocalAddr, routerConn.LocalAddr))
		}
		if ctrlConn.RemoteAddr != routerConn.RemoteAddr {
			result.IsValid = false
			result.Messages = append(result.Messages, fmt.Sprintf("for link %s, remote addr doesn't match. ctrl %s != router %s",
				ctrlLink.Id, ctrlConn.RemoteAddr, routerConn.RemoteAddr))
		}
	}
}

func (network *Network) reportRouterLinksError(router *model.Router, err error, cb LinkValidationCallback) {
	result := &mgmt_pb.RouterLinkDetails{
		RouterId:        router.Id,
		RouterName:      router.Name,
		ValidateSuccess: false,
		Message:         err.Error(),
	}
	cb(result)
}

func minCost(q map[*model.Router]bool, dist map[*model.Router]int64) *model.Router {
	if len(dist) < 1 {
		return nil
	}

	currentMin := int64(math.MaxInt64)
	var selected *model.Router
	for r := range q {
		d := dist[r]
		if d <= currentMin {
			selected = r
			currentMin = d
		}
	}
	return selected
}

type Cache interface {
	RemoveFromCache(id string)
}

func newPathAndCost(path []*model.Router, cost int64) *PathAndCost {
	if cost > (1 << 20) {
		cost = 1 << 20
	}
	return &PathAndCost{
		path: path,
		cost: uint32(cost),
	}
}

type PathAndCost struct {
	path []*model.Router
	cost uint32
}

type InvalidCircuitError struct {
	circuitId string
}

func (err InvalidCircuitError) Error() string {
	return fmt.Sprintf("invalid circuit (%s)", err.circuitId)
}
