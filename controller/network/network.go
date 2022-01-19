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

package network

import (
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/channel"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/event"
	"github.com/openziti/foundation/events"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/debugz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/foundation/util/sequence"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	"sort"
	"strings"
	"sync"
	"time"
)

const SmartRerouteAttempt = 99969996

type Network struct {
	*Controllers
	nodeId                 string
	options                *Options
	routerChanged          chan *Router
	linkController         *linkController
	linkChanged            chan *Link
	forwardingFaults       chan *ForwardingFaultReport
	circuitController      *circuitController
	routeSenderController  *routeSenderController
	sequence               *sequence.Sequence
	eventDispatcher        event.Dispatcher
	traceController        trace.Controller
	routerPresenceHandlers []RouterPresenceHandler
	capabilities           []string
	closeNotify            <-chan struct{}
	lock                   sync.Mutex
	strategyRegistry       xt.Registry
	lastSnapshot           time.Time
	metricsRegistry        metrics.Registry
	VersionProvider        common.VersionProvider

	serviceEventMetrics          metrics.UsageRegistry
	serviceDialSuccessCounter    metrics.IntervalCounter
	serviceDialFailCounter       metrics.IntervalCounter
	serviceDialTimeoutCounter    metrics.IntervalCounter
	serviceDialOtherErrorCounter metrics.IntervalCounter
}

func NewNetwork(nodeId string, options *Options, database boltz.Db, metricsCfg *metrics.Config, versionProvider common.VersionProvider, closeNotify <-chan struct{}) (*Network, error) {
	stores, err := db.InitStores(database)
	if err != nil {
		return nil, err
	}

	controllers := NewControllers(database, stores)

	serviceEventMetrics := metrics.NewUsageRegistry(nodeId, nil, closeNotify)

	network := &Network{
		Controllers:           controllers,
		nodeId:                nodeId,
		options:               options,
		routerChanged:         make(chan *Router, 16),
		linkController:        newLinkController(),
		linkChanged:           make(chan *Link, 16),
		forwardingFaults:      make(chan *ForwardingFaultReport, 16),
		circuitController:     newCircuitController(),
		routeSenderController: newRouteSenderController(),
		sequence:              sequence.NewSequence(),
		eventDispatcher:       event.NewDispatcher(closeNotify),
		traceController:       trace.NewController(closeNotify),
		closeNotify:           closeNotify,
		strategyRegistry:      xt.GlobalRegistry(),
		lastSnapshot:          time.Now().Add(-time.Hour),
		metricsRegistry:       metrics.NewRegistry(nodeId, nil),
		VersionProvider:       versionProvider,

		serviceEventMetrics:          serviceEventMetrics,
		serviceDialSuccessCounter:    serviceEventMetrics.IntervalCounter("service.dial.success", time.Minute),
		serviceDialFailCounter:       serviceEventMetrics.IntervalCounter("service.dial.fail", time.Minute),
		serviceDialTimeoutCounter:    serviceEventMetrics.IntervalCounter("service.dial.timeout", time.Minute),
		serviceDialOtherErrorCounter: serviceEventMetrics.IntervalCounter("service.dial.error_other", time.Minute),
	}

	network.Controllers.Inspections.network = network

	metrics.Init(metricsCfg)
	events.AddMetricsEventHandler(network)
	network.AddCapability("ziti.fabric")
	network.showOptions()
	network.relayControllerMetrics(metricsCfg)
	return network, nil
}

func (network *Network) relayControllerMetrics(cfg *metrics.Config) {
	reportInterval := time.Minute
	if cfg != nil && cfg.ReportInterval != 0 {
		reportInterval = cfg.ReportInterval
	}
	go func() {
		timer := time.NewTicker(reportInterval)
		defer timer.Stop()

		dispatcher := metrics.NewDispatchWrapper(network.eventDispatcher.Dispatch)
		for {
			select {
			case <-timer.C:
				if msg := network.metricsRegistry.Poll(); msg != nil {
					dispatcher.AcceptMetrics(msg)
				}
			case <-network.closeNotify:
				return
			}
		}
	}()
}

func (network *Network) InitServiceCounterDispatch(handler metrics.Handler) {
	network.serviceEventMetrics.StartReporting(handler, time.Minute, 10)
}

func (network *Network) GetAppId() string {
	return network.nodeId
}

func (network *Network) GetOptions() *Options {
	return network.options
}

func (network *Network) GetDb() boltz.Db {
	return network.db
}

func (network *Network) GetStores() *db.Stores {
	return network.stores
}

func (network *Network) GetControllers() *Controllers {
	return network.Controllers
}

func (network *Network) CreateRouter(router *Router) error {
	return network.Routers.Create(router)
}

func (network *Network) GetConnectedRouter(routerId string) *Router {
	return network.Routers.getConnected(routerId)
}

func (network *Network) GetRouter(routerId string) (*Router, error) {
	return network.Routers.Read(routerId)
}

func (network *Network) AllConnectedRouters() []*Router {
	return network.Routers.allConnected()
}

func (network *Network) GetLink(linkId string) (*Link, bool) {
	return network.linkController.get(linkId)
}

func (network *Network) GetAllLinks() []*Link {
	return network.linkController.all()
}

func (network *Network) GetAllLinksForRouter(routerId string) []*Link {
	r := network.GetConnectedRouter(routerId)
	if r == nil {
		return nil
	}
	return r.routerLinks.GetLinks()
}

func (network *Network) GetCircuit(circuitId string) (*Circuit, bool) {
	return network.circuitController.get(circuitId)
}

func (network *Network) GetAllCircuits() []*Circuit {
	return network.circuitController.all()
}

func (network *Network) RouteResult(r *Router, circuitId string, attempt uint32, success bool, rerr string, peerData xt.PeerData) bool {
	return network.routeSenderController.forwardRouteResult(r, circuitId, attempt, success, rerr, peerData)
}

func (network *Network) newRouteSender(circuitId string) *routeSender {
	rs := newRouteSender(circuitId, network.options.RouteTimeout, network)
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

func (network *Network) RouterChanged(r *Router) {
	network.routerChanged <- r
}

func (network *Network) ConnectedRouter(id string) bool {
	return network.Routers.IsConnected(id)
}

func (network *Network) ConnectRouter(r *Router) {
	network.Routers.markConnected(r)
	network.routerChanged <- r

	for _, h := range network.routerPresenceHandlers {
		h.RouterConnected(r)
	}
}

func (network *Network) ValidateTerminators(r *Router) {
	result, err := network.Terminators.Query(fmt.Sprintf(`router.id = "%v" limit none`, r.Id))
	if err != nil {
		pfxlog.Logger().Errorf("failed to get termintors for router %v (%v)", r.Id, err)
		return
	}

	pfxlog.Logger().Debugf("%v terminators on %v to validate", len(result.Entities), r.Id)
	if len(result.Entities) == 0 {
		return
	}

	var terminators []*ctrl_pb.Terminator

	for _, terminator := range result.Entities {
		terminators = append(terminators, &ctrl_pb.Terminator{
			Id:      terminator.Id,
			Binding: terminator.Binding,
			Address: terminator.Address,
		})
	}

	req := &ctrl_pb.ValidateTerminatorsRequest{
		Terminators: terminators,
	}

	if err = r.Control.Send(channel.MarshalTyped(req)); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error sending ValidateTerminatorsRequest")
	}
}

func (network *Network) DisconnectRouter(r *Router) {
	// 1: remove Links for Router
	for _, l := range r.routerLinks.GetLinks() {
		network.linkController.remove(l)
		network.LinkChanged(l)
	}
	// 2: remove Router
	network.Routers.markDisconnected(r)
	network.routerChanged <- r

	for _, h := range network.routerPresenceHandlers {
		h.RouterDisconnected(r)
	}
}

func (network *Network) LinkConnected(id string, connected bool) error {
	log := pfxlog.Logger().WithField("linkId", id)

	if l, found := network.linkController.get(id); found {
		if connected {
			l.addState(newLinkState(Connected))
			log.Info("link connected")
			return nil
		}
		l.addState(newLinkState(Failed))
		log.Info("link failed")
		return nil
	}
	return errors.Errorf("no such link [l/%s]", id)
}

func (network *Network) VerifyLinkSource(targetRouter *Router, linkId string, fingerprints []string) error {
	l, found := network.linkController.get(linkId)
	if !found {
		return errors.Errorf("invalid link %v", linkId)
	}

	if l.Dst.Id != targetRouter.Id {
		return errors.Errorf("incorrect link target router. link=%v, expected router=%v, actual router=%v", l.Id, l.Dst.Id, targetRouter.Id)
	}

	routerFingerprint := l.Src.Fingerprint
	if routerFingerprint == nil {
		return errors.Errorf("invalid source router %v for link %v, not yet enrolled", l.Src.Id, l.Id)
	}

	for _, fp := range fingerprints {
		if fp == *routerFingerprint {
			return nil
		}
	}

	return errors.Errorf("could not verify fingerprint for router %v on link %v", l.Src.Id, l.Id)
}

func (network *Network) LinkChanged(l *Link) {
	// This is called from Channel.rxer() and thus may not block
	go func() {
		network.linkChanged <- l
	}()
}

func (network *Network) CreateCircuit(srcR *Router, clientId *identity.TokenId, service string, ctx logcontext.Context) (*Circuit, error) {
	// 1: Allocate Circuit Identifier
	circuitId, err := network.circuitController.nextCircuitId()
	if err != nil {
		return nil, err
	}
	ctx.WithField("circuitId", circuitId)
	ctx.WithField("attemptNumber", 1)
	logger := pfxlog.ChannelLogger(logcontext.SelectPath).Wire(ctx).Entry
	targetIdentity, serviceId := parseIdentityAndService(service)

	attempt := uint32(0)
	allCleanups := make(map[string]struct{})
	rs := network.newRouteSender(circuitId)
	defer func() { network.removeRouteSender(rs) }()
	for {
		// 2: Find Service
		svc, err := network.Services.Read(serviceId)
		if err != nil {
			network.ServiceDialOtherError(serviceId)
			return nil, err
		}

		// 3: select terminator
		strategy, terminator, pathNodes, err := network.selectPath(srcR, svc, targetIdentity, ctx)
		if err != nil {
			network.ServiceDialOtherError(serviceId)
			return nil, err
		}

		// 4: Create Path
		path, err := network.CreatePathWithNodes(pathNodes)
		if err != nil {
			network.ServiceDialOtherError(serviceId)
			return nil, err
		}

		// 4a: Create Route Messages
		rms := path.CreateRouteMessages(attempt, circuitId, terminator)
		rms[len(rms)-1].Egress.PeerData = clientId.Data

		for _, msg := range rms {
			msg.Context = &ctrl_pb.Context{
				Fields:      ctx.GetStringFields(),
				ChannelMask: ctx.GetChannelsMask(),
			}
		}

		// 5: Routing
		logger.Debug("route attempt for circuit")
		peerData, cleanups, err := rs.route(attempt, path, rms, strategy, terminator, ctx)
		for k, v := range cleanups {
			allCleanups[k] = v
		}
		if err != nil {
			logger.WithError(err).Warn("route attempt for circuit failed")
			attempt++
			ctx.WithField("attemptNumber", attempt+1)
			logger = logger.WithField("attemptNumber", attempt+1)
			if attempt < network.options.CreateCircuitRetries {
				continue
			} else {
				// revert successful routes
				logger.Warnf("circuit creation failed after [%d] attempts, sending cleanup unroutes", network.options.CreateCircuitRetries)
				for cleanupRId := range allCleanups {
					if r, err := network.GetRouter(cleanupRId); err == nil {
						if err := sendUnroute(r, circuitId, true); err == nil {
							logger.WithField("routerId", cleanupRId).Debug("sent cleanup unroute for circuit")
						} else {
							logger.WithField("routerId", cleanupRId).Error("error sending cleanup unroute for circuit")
						}
					} else {
						logger.WithField("routerId", cleanupRId).Error("missing router for circuit cleanup")
					}
				}

				return nil, errors.Wrapf(err, "exceeded maximum [%d] retries creating circuit [c/%s]", network.options.CreateCircuitRetries, circuitId)
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
				if r, err := network.GetRouter(cleanupRId); err == nil {
					if err := sendUnroute(r, circuitId, true); err == nil {
						logger.WithField("routerId", cleanupRId).Debug("sent abandoned cleanup unroute for circuit to router")
					} else {
						logger.WithField("routerId", cleanupRId).WithError(err).Error("error sending abandoned cleanup unroute for circuit to router")
					}
				} else {
					logger.WithField("routerId", cleanupRId).Error("missing router for circuit, abandoned cleanup")
				}
			}
		}
		logger.Debugf("cleaned up [%d] abandoned routers for circuit", cleanupCount)

		// 6: Create Circuit Object
		circuit := &Circuit{
			Id:         circuitId,
			ClientId:   clientId.Token,
			Service:    svc,
			Path:       path,
			Terminator: terminator,
			PeerData:   peerData,
		}
		network.circuitController.add(circuit)
		network.CircuitCreated(circuit.Id, circuit.ClientId, circuit.Service.Id, circuit.Path)

		logger.WithField("path", circuit.Path).Debug("created circuit")
		return circuit, nil
	}
}

func (network *Network) ReportForwardingFaults(ffr *ForwardingFaultReport) {
	network.forwardingFaults <- ffr
}

func parseIdentityAndService(service string) (string, string) {
	atIndex := strings.IndexRune(service, '@')
	if atIndex < 0 {
		return "", service
	}
	identityId := service[0:atIndex]
	serviceId := service[atIndex+1:]
	return identityId, serviceId
}

func (network *Network) selectPath(srcR *Router, svc *Service, identity string, ctx logcontext.Context) (xt.Strategy, xt.Terminator, []*Router, error) {
	paths := map[string]*PathAndCost{}
	var weightedTerminators []xt.CostedTerminator
	var errList []error

	log := pfxlog.ChannelLogger(logcontext.SelectPath).Wire(ctx)

	for _, terminator := range svc.Terminators {
		if terminator.Identity != identity {
			continue
		}

		pathAndCost, found := paths[terminator.Router]
		if !found {
			dstR := network.Routers.getConnected(terminator.GetRouterId())
			if dstR == nil {
				err := errors.Errorf("router with id=%v on terminator with id=%v for service name=%v is not online",
					terminator.GetRouterId(), terminator.GetId(), svc.Name)
				log.Debugf("error while calculating path for service %v: %v", svc.Id, err)
				errList = append(errList, err)
				continue
			}

			path, cost, err := network.shortestPath(srcR, dstR)
			if err != nil {
				log.Debugf("error while calculating path for service %v: %v", svc.Id, err)
				errList = append(errList, err)
				continue
			}

			pathAndCost = newPathAndCost(path, cost)
			paths[terminator.GetRouterId()] = pathAndCost
		}

		dynamicCost := xt.GlobalCosts().GetDynamicCost(terminator.Id)
		unbiasedCost := uint32(terminator.Cost) + uint32(dynamicCost) + pathAndCost.cost
		biasedCost := terminator.Precedence.GetBiasedCost(unbiasedCost)
		costedTerminator := &RoutingTerminator{
			Terminator: terminator,
			RouteCost:  biasedCost,
		}
		weightedTerminators = append(weightedTerminators, costedTerminator)
	}

	if len(svc.Terminators) == 0 {
		return nil, nil, nil, errors.Errorf("service %v has no terminators", svc.Id)
	}

	if len(weightedTerminators) == 0 && len(errList) == 0 {
		return nil, nil, nil, errors.Errorf("service %v has no terminators for identity %v", svc.Id, identity)
	}

	if len(weightedTerminators) == 0 {
		return nil, nil, nil, errorz.MultipleErrors(errList)
	}

	strategy, err := network.strategyRegistry.GetStrategy(svc.TerminatorStrategy)
	if err != nil {
		return nil, nil, nil, err
	}

	sort.Slice(weightedTerminators, func(i, j int) bool {
		return weightedTerminators[i].GetRouteCost() < weightedTerminators[j].GetRouteCost()
	})

	terminator, err := strategy.Select(weightedTerminators)

	if err != nil {
		return nil, nil, nil, errors.Errorf("strategy %v errored selecting terminator for service %v: %v", svc.TerminatorStrategy, svc.Id, err)
	}

	if terminator == nil {
		return nil, nil, nil, errors.Errorf("strategy %v did not select terminator for service %v", svc.TerminatorStrategy, svc.Id)
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

	return strategy, terminator, path, nil
}

func (network *Network) RemoveCircuit(circuitId string, now bool) error {
	log := pfxlog.Logger()

	if ss, found := network.circuitController.get(circuitId); found {
		for _, r := range ss.Path.Nodes {
			err := sendUnroute(r, ss.Id, now)
			if err != nil {
				log.Errorf("error sending unroute to [r/%s] (%s)", r.Id, err)
			}
		}
		network.circuitController.remove(ss)
		network.CircuitDeleted(ss.Id, ss.ClientId)

		if strategy, err := network.strategyRegistry.GetStrategy(ss.Service.TerminatorStrategy); strategy != nil {
			strategy.NotifyEvent(xt.NewCircuitRemoved(ss.Terminator))
		} else if err != nil {
			log.Warnf("failed to notify strategy %v of circuit end. invalid strategy (%v)", ss.Service.TerminatorStrategy, err)
		}

		log.Debugf("removed circuit [s/%s]", ss.Id)

		return nil
	}
	return InvalidCircuitError{circuitId: circuitId}
}

func (network *Network) CreatePath(srcR, dstR *Router) (*Path, error) {
	ingressId, err := network.sequence.NextHash()
	if err != nil {
		return nil, err
	}

	egressId, err := network.sequence.NextHash()
	if err != nil {
		return nil, err
	}

	path := &Path{
		Links:     make([]*Link, 0),
		IngressId: ingressId,
		EgressId:  egressId,
		Nodes:     make([]*Router, 0),
	}
	path.Nodes = append(path.Nodes, srcR)
	path.Nodes = append(path.Nodes, dstR)

	return network.UpdatePath(path)
}

func (network *Network) CreatePathWithNodes(nodes []*Router) (*Path, error) {
	ingressId, err := network.sequence.NextHash()
	if err != nil {
		return nil, err
	}

	egressId, err := network.sequence.NextHash()
	if err != nil {
		return nil, err
	}

	path := &Path{
		Nodes:     nodes,
		IngressId: ingressId,
		EgressId:  egressId,
	}
	if err := network.setLinks(path); err != nil {
		return nil, err
	}
	return path, nil
}

func (network *Network) UpdatePath(path *Path) (*Path, error) {
	srcR := path.Nodes[0]
	dstR := path.Nodes[len(path.Nodes)-1]
	nodes, _, err := network.shortestPath(srcR, dstR)
	if err != nil {
		return nil, err
	}

	path2 := &Path{
		Nodes:     nodes,
		IngressId: path.IngressId,
		EgressId:  path.EgressId,
	}
	if err := network.setLinks(path2); err != nil {
		return nil, err
	}
	return path2, nil
}

func (network *Network) setLinks(path *Path) error {
	if len(path.Nodes) > 1 {
		for i := 0; i < len(path.Nodes)-1; i++ {
			if link, found := network.linkController.leastExpensiveLink(path.Nodes[i], path.Nodes[i+1]); found {
				path.Links = append(path.Links, link)
			} else {
				return errors.Errorf("no link from r/%v to r/%v", path.Nodes[i].Id, path.Nodes[i+1].Id)
			}
		}
	}
	return nil
}

func (network *Network) AddRouterPresenceHandler(h RouterPresenceHandler) {
	network.routerPresenceHandlers = append(network.routerPresenceHandlers, h)
}

func (network *Network) Run() {
	defer logrus.Error("exited")
	logrus.Info("started")

	for {
		select {
		case r := <-network.routerChanged:
			logrus.WithField("routerId", r.Id).Info("changed router")
			network.assemble()
			network.clean()

		case l := <-network.linkChanged:
			go network.handleLinkChanged(l)

		case ffr := <-network.forwardingFaults:
			go network.handleForwardingFaults(ffr)
			network.clean()

		case <-time.After(time.Duration(network.options.CycleSeconds) * time.Second):
			network.assemble()
			network.clean()
			network.smart()

		case <-network.closeNotify:
			events.RemoveMetricsEventHandler(network)
			network.metricsRegistry.DisposeAll()
			return
		}
	}
}

func (network *Network) handleLinkChanged(l *Link) {
	log := logrus.WithField("linkId", l.Id)
	log.Info("changed link")
	if err := network.rerouteLink(l); err != nil {
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

func (network *Network) rerouteLink(l *Link) error {
	circuits := network.circuitController.all()
	for _, circuit := range circuits {
		if circuit.Path.usesLink(l) {
			log := logrus.WithField("linkId", l.Id).
				WithField("circuitId", circuit.Id)
			log.Info("circuit uses link")
			if err := network.rerouteCircuit(circuit); err != nil {
				log.WithError(err).Error("error rerouting circuit, removing")
				if err := network.RemoveCircuit(circuit.Id, true); err != nil {
					log.WithError(err).Error("error removing circuit after reroute failure")
				}
			}
		}
	}

	return nil
}

func (network *Network) rerouteCircuit(circuit *Circuit) error {
	if circuit.Rerouting.CompareAndSwap(false, true) {
		defer circuit.Rerouting.Set(false)

		logrus.Warnf("rerouting [s/%s]", circuit.Id)

		if cq, err := network.UpdatePath(circuit.Path); err == nil {
			circuit.Path = cq

			rms := cq.CreateRouteMessages(SmartRerouteAttempt, circuit.Id, circuit.Terminator)

			for i := 0; i < len(cq.Nodes); i++ {
				if _, err := sendRoute(cq.Nodes[i], rms[i], network.options.RouteTimeout); err != nil {
					logrus.Errorf("error sending route to [r/%s] (%s)", cq.Nodes[i].Id, err)
				}
			}

			logrus.Infof("rerouted circuit [s/%s]", circuit.Id)

			network.PathUpdated(circuit.Id, circuit.Path)

			return nil
		} else {
			return err
		}
	} else {
		logrus.Infof("not rerouting [s/%s], already in progress", circuit.Id)
		return nil
	}
}

func (network *Network) smartReroute(s *Circuit, cq *Path) error {
	if s.Rerouting.CompareAndSwap(false, true) {
		defer s.Rerouting.Set(false)

		s.Path = cq

		rms := cq.CreateRouteMessages(SmartRerouteAttempt, s.Id, s.Terminator)

		for i := 0; i < len(cq.Nodes); i++ {
			if _, err := sendRoute(cq.Nodes[i], rms[i], network.options.RouteTimeout); err != nil {
				logrus.Errorf("error sending route to [r/%s] (%s)", cq.Nodes[i].Id, err)
			}
		}

		logrus.Debugf("rerouted circuit [s/%s]", s.Id)

		network.PathUpdated(s.Id, s.Path)

		return nil

	} else {
		logrus.Infof("not rerouting [s/%s], already in progress", s.Id)
		return nil
	}
}

func (network *Network) AcceptMetrics(metrics *metrics_pb.MetricsMessage) {
	if metrics.SourceId == network.nodeId {
		return // ignore metrics coming from the controller itself
	}

	log := pfxlog.Logger()

	router, err := network.Routers.Read(metrics.SourceId)
	if err != nil {
		log.Debugf("could not find router [r/%s] while processing metrics", metrics.SourceId)
		return
	}

	for _, link := range network.GetAllLinksForRouter(router.Id) {
		metricId := "link." + link.Id + ".latency"
		if latency, ok := metrics.Histograms[metricId]; ok {
			if link.Src.Id == router.Id {
				link.SetSrcLatency(int64(latency.Mean)) // latency is in nanoseconds
			} else if link.Dst.Id == router.Id {
				link.SetDstLatency(int64(latency.Mean)) // latency is in nanoseconds
			} else {
				log.Warnf("link not for router")
			}
		}
	}
}

func sendRoute(r *Router, createMsg *ctrl_pb.Route, timeout time.Duration) (xt.PeerData, error) {
	log := pfxlog.Logger().WithField("routerId", r.Id).
		WithField("circuitId", createMsg.CircuitId)

	log.Debug("sending create route message")

	msg, err := channel.MarshalTyped(createMsg).WithTimeout(timeout).SendForReply(r.Control)
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

func sendUnroute(r *Router, circuitId string, now bool) error {
	unroute := &ctrl_pb.Unroute{
		CircuitId: circuitId,
		Now:       now,
	}
	return r.Control.Send(channel.MarshalTyped(unroute))
}

func (network *Network) showOptions() {
	if jsonOptions, err := json.MarshalIndent(network.options, "", "  "); err == nil {
		pfxlog.Logger().Infof("network = %s", string(jsonOptions))
	} else {
		panic(err)
	}
}

func (network *Network) GetServiceCache() Cache {
	return network.Services
}

func (network *Network) NotifyRouterRenamed(id, name string) {
	if cached, _ := network.Routers.cache.Get(id); cached != nil {
		if cachedRouter, ok := cached.(*Router); ok {
			cachedRouter.Name = name
		}
	}

	if cached, _ := network.Routers.connected.Get(id); cached != nil {
		if cachedRouter, ok := cached.(*Router); ok {
			cachedRouter.Name = name
		}
	}
}

func (network *Network) Inspect(name string) *string {
	if strings.ToLower(name) == "stackdump" {
		result := debugz.GenerateStack()
		return &result
	}
	return nil
}

var DbSnapshotTooFrequentError = dbSnapshotTooFrequentError{}

type dbSnapshotTooFrequentError struct{}

func (d dbSnapshotTooFrequentError) Error() string {
	return "may snapshot database at most once per minute"
}

func (network *Network) SnapshotDatabase() error {
	network.lock.Lock()
	defer network.lock.Unlock()

	if network.lastSnapshot.Add(time.Minute).After(time.Now()) {
		return DbSnapshotTooFrequentError
	}
	pfxlog.Logger().Info("snapshotting database")
	err := network.GetDb().View(func(tx *bbolt.Tx) error {
		return network.GetDb().Snapshot(tx)
	})
	if err == nil {
		network.lastSnapshot = time.Now()
	}
	return err
}

type Cache interface {
	RemoveFromCache(id string)
}

func newPathAndCost(path []*Router, cost int64) *PathAndCost {
	if cost > (1 << 20) {
		cost = 1 << 20
	}
	return &PathAndCost{
		path: path,
		cost: uint32(cost),
	}
}

type PathAndCost struct {
	path []*Router
	cost uint32
}

type InvalidCircuitError struct {
	circuitId string
}

func (err InvalidCircuitError) Error() string {
	return fmt.Sprintf("invalid circuit (%s)", err.circuitId)
}
