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
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/event"
	"github.com/openziti/foundation/events"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/openziti/foundation/storage/boltz"
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
	nodeId                 *identity.TokenId
	options                *Options
	routerChanged          chan *Router
	linkController         *linkController
	linkChanged            chan *Link
	forwardingFaults       chan *ForwardingFaultReport
	sessionController      *sessionController
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
}

func NewNetwork(nodeId *identity.TokenId, options *Options, database boltz.Db, metricsCfg *metrics.Config, versionProvider common.VersionProvider, closeNotify <-chan struct{}) (*Network, error) {
	stores, err := db.InitStores(database)
	if err != nil {
		return nil, err
	}

	controllers := NewControllers(database, stores)

	network := &Network{
		Controllers:           controllers,
		nodeId:                nodeId,
		options:               options,
		routerChanged:         make(chan *Router, 16),
		linkController:        newLinkController(),
		linkChanged:           make(chan *Link, 16),
		forwardingFaults:      make(chan *ForwardingFaultReport, 16),
		sessionController:     newSessionController(),
		routeSenderController: newRouteSenderController(),
		sequence:              sequence.NewSequence(),
		eventDispatcher:       event.NewDispatcher(closeNotify),
		traceController:       trace.NewController(closeNotify),
		closeNotify:           closeNotify,
		strategyRegistry:      xt.GlobalRegistry(),
		lastSnapshot:          time.Now().Add(-time.Hour),
		metricsRegistry:       metrics.NewRegistry(nodeId.Token, nil),
		VersionProvider:       versionProvider,
	}
	metrics.Init(metricsCfg)
	events.AddMetricsEventHandler(network)
	network.AddCapability("ziti.fabric")
	network.showOptions()
	network.relayControllerMetrics(metricsCfg)
	return network, nil
}

func (network *Network) relayControllerMetrics(cfg *metrics.Config) {
	reportInterval := time.Second * 15
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

func (network *Network) GetAppId() *identity.TokenId {
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

func (network *Network) GetLink(linkId *identity.TokenId) (*Link, bool) {
	return network.linkController.get(linkId)
}

func (network *Network) GetAllLinks() []*Link {
	return network.linkController.all()
}

func (network *Network) GetAllLinksForRouter(routerId string) []*Link {
	return network.linkController.allLinksForRouter(routerId)
}

func (network *Network) GetSession(sessionId *identity.TokenId) (*Session, bool) {
	return network.sessionController.get(sessionId)
}

func (network *Network) GetAllSessions() []*Session {
	return network.sessionController.all()
}

func (network *Network) RouteResult(r *Router, sessionId string, attempt uint32, success bool, rerr string, peerData xt.PeerData) bool {
	return network.routeSenderController.forwardRouteResult(r, sessionId, attempt, success, rerr, peerData)
}

func (network *Network) newRouteSender(sessionId string) *routeSender {
	rs := newRouteSender(sessionId, network.options.RouteTimeout)
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

	body, err := proto.Marshal(req)
	if err != nil {
		pfxlog.Logger().Errorf("unexpected error serializing ValidateTerminatorsRequest (%s)", err)
		return
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_ValidateTerminatorsRequestType), body)
	if err := r.Control.Send(msg); err != nil {
		pfxlog.Logger().Errorf("unexpected error sending ValidateTerminatorsRequest (%s)", err)
	}
}

func (network *Network) DisconnectRouter(r *Router) {
	// 1: remove Links for Router
	for _, l := range network.linkController.allLinksForRouter(r.Id) {
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

func (network *Network) LinkConnected(id *identity.TokenId, connected bool) error {
	log := pfxlog.Logger()

	if l, found := network.linkController.get(id); found {
		if connected {
			l.addState(newLinkState(Connected))
			log.Infof("link [l/%s] connected", id.Token)
			return nil
		}
		l.addState(newLinkState(Failed))
		log.Infof("link [l/%s] failed", id.Token)
		return nil
	}
	return fmt.Errorf("no such link [l/%s]", id.Token)
}

func (network *Network) LinkChanged(l *Link) {
	// This is called from Channel.rxer() and thus may not block
	go func() {
		network.linkChanged <- l
	}()
}

func (network *Network) CreateSession(srcR *Router, clientId *identity.TokenId, service string) (*Session, error) {
	// 1: Allocate Session Identifier
	sessionIdHash, err := network.sequence.NextHash()
	if err != nil {
		return nil, err
	}
	sessionId := &identity.TokenId{Token: sessionIdHash}

	targetIdentity, serviceId := parseIdentityAndService(service)

	attempt := uint32(0)
	allCleanups := make(map[string]struct{})
	rs := network.newRouteSender(sessionId.Token)
	defer func() { network.removeRouteSender(rs) }()
	for {
		// 2: Find Service
		svc, err := network.Services.Read(serviceId)
		if err != nil {
			return nil, err
		}

		// 3: select terminator
		strategy, terminator, path, err := network.selectPath(srcR, svc, targetIdentity)
		if err != nil {
			return nil, err
		}

		// 4: Create Circuit
		circuit, err := network.CreateCircuitWithPath(path)
		if err != nil {
			return nil, err
		}
		circuit.Binding = "transport"
		if terminator.GetBinding() != "" {
			circuit.Binding = terminator.GetBinding()
		} else if strings.HasPrefix(terminator.GetBinding(), "hosted") {
			circuit.Binding = "edge"
		} else if strings.HasPrefix(terminator.GetAddress(), "udp") {
			circuit.Binding = "udp"
		}

		// 4a: Create Route Messages
		rms, err := circuit.CreateRouteMessages(attempt, sessionId, terminator.GetAddress())
		if err != nil {
			return nil, err
		}
		rms[len(rms)-1].Egress.PeerData = clientId.Data

		// 5: Routing
		logrus.Debugf("route attempt [#%d] for [s/%s]", attempt+1, sessionId.Token)
		peerData, cleanups, err := rs.route(attempt, circuit, rms, strategy, terminator)
		for k, v := range cleanups {
			allCleanups[k] = v
		}
		if err != nil {
			logrus.Warnf("route attempt [#%d] for [s/%s] failed (%v)", attempt+1, sessionId.Token, err)
			attempt++
			if attempt < network.options.CreateSessionRetries {
				continue

			} else {
				// revert successful routes
				logrus.Warnf("session creation failed after [%d] attempts, sending cleanup unroutes for [s/%s]", network.options.CreateSessionRetries, sessionId.Token)
				for cleanupRId, _ := range allCleanups {
					if r, err := network.GetRouter(cleanupRId); err == nil {
						if err := sendUnroute(r, sessionId, true); err == nil {
							logrus.Debugf("sent cleanup unroute for [s/%s] to [r/%s]", sessionId.Token, r.Id)
						} else {
							logrus.Errorf("error sending cleanup unroute for [s/%s] to [r/%s]", sessionId.Token, r.Id)
						}
					} else {
						logrus.Errorf("missing [r/%s] for [s/%s] cleanup", r.Id, sessionId.Token)
					}
				}

				return nil, errors.Wrapf(err, "exceeded maximum [%d] retries creating session [s/%s]", network.options.CreateSessionRetries, sessionId.Token)
			}
		}

		// 5.a: Unroute Abandoned Routers (from Previous Attempts)
		usedRouters := make(map[string]struct{})
		for _, r := range circuit.Path {
			usedRouters[r.Id] = struct{}{}
		}
		cleanupCount := 0
		for cleanupRId, _ := range allCleanups {
			if _, found := usedRouters[cleanupRId]; !found {
				cleanupCount++
				if r, err := network.GetRouter(cleanupRId); err == nil {
					if err := sendUnroute(r, sessionId, true); err == nil {
						logrus.Debugf("sent abandoned cleanup unroute for [s/%s] to [r/%s]", sessionId.Token, r.Id)
					} else {
						logrus.Errorf("error sending abandoned cleanup unroute for [s/%s] to [r/%s]", sessionId.Token, r.Id)
					}
				} else {
					logrus.Errorf("missing [r/%s] for [s/%s] abandoned cleanup", r.Id, sessionId.Token)
				}
			}
		}
		logrus.Debugf("cleaned up [%d] abandoned routers for [s/%s]", cleanupCount, sessionId.Token)

		// 6: Create Session Object
		ss := &Session{
			Id:         sessionId,
			ClientId:   clientId,
			Service:    svc,
			Circuit:    circuit,
			Terminator: terminator,
			PeerData:   peerData,
		}
		network.sessionController.add(ss)
		network.SessionCreated(ss.Id, ss.ClientId, ss.Service.Id, ss.Circuit)

		logrus.Debugf("created session [s/%s] ==> %s", sessionId.Token, ss.Circuit)
		return ss, nil
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
	identity := service[0:atIndex]
	serviceId := service[atIndex+1:]
	return identity, serviceId
}

func (network *Network) selectPath(srcR *Router, svc *Service, identity string) (xt.Strategy, xt.Terminator, []*Router, error) {
	paths := map[string]*PathAndCost{}
	var weightedTerminators []xt.CostedTerminator
	var errList []error

	log := pfxlog.Logger()

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
				pfxlog.Logger().Debugf("error while calculating path for service %v: %v", svc.Id, err)
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
		return nil, nil, nil, MultipleErrors(errList)
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

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
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
		pfxlog.Logger().Infof("selected %v for path from %v", terminator.GetId(), buf.String())
	}

	return strategy, terminator, paths[terminator.GetRouterId()].path, nil
}

func (network *Network) RemoveSession(sessionId *identity.TokenId, now bool) error {
	log := pfxlog.Logger()

	if ss, found := network.sessionController.get(sessionId); found {
		for _, r := range ss.Circuit.Path {
			err := sendUnroute(r, ss.Id, now)
			if err != nil {
				log.Errorf("error sending unroute to [r/%s] (%s)", r.Id, err)
			}
		}
		network.sessionController.remove(ss)
		network.SessionDeleted(ss.Id, ss.ClientId)

		if strategy, err := network.strategyRegistry.GetStrategy(ss.Service.TerminatorStrategy); strategy != nil {
			strategy.NotifyEvent(xt.NewSessionEnded(ss.Terminator))
		} else if err != nil {
			log.Warnf("failed to notify strategy %v of session end. invalid strategy (%v)", ss.Service.TerminatorStrategy, err)
		}

		log.Debugf("removed session [s/%s]", ss.Id.Token)

		return nil
	}
	return InvalidSessionError{sessionId: sessionId.Token}
}

func (network *Network) CreateCircuit(srcR, dstR *Router) (*Circuit, error) {
	ingressId, err := network.sequence.NextHash()
	if err != nil {
		return nil, err
	}

	egressId, err := network.sequence.NextHash()
	if err != nil {
		return nil, err
	}

	circuit := &Circuit{
		Links:     make([]*Link, 0),
		IngressId: ingressId,
		EgressId:  egressId,
		Path:      make([]*Router, 0),
	}
	circuit.Path = append(circuit.Path, srcR)
	circuit.Path = append(circuit.Path, dstR)

	return network.UpdateCircuit(circuit)
}

func (network *Network) CreateCircuitWithPath(path []*Router) (*Circuit, error) {
	ingressId, err := network.sequence.NextHash()
	if err != nil {
		return nil, err
	}

	egressId, err := network.sequence.NextHash()
	if err != nil {
		return nil, err
	}

	circuit := &Circuit{
		Path:      path,
		IngressId: ingressId,
		EgressId:  egressId,
	}
	if err := network.setLinks(circuit); err != nil {
		return nil, err
	}
	return circuit, nil
}

func (network *Network) UpdateCircuit(circuit *Circuit) (*Circuit, error) {
	srcR := circuit.Path[0]
	dstR := circuit.Path[len(circuit.Path)-1]
	path, _, err := network.shortestPath(srcR, dstR)
	if err != nil {
		return nil, err
	}

	circuit2 := &Circuit{
		Path:      path,
		Binding:   circuit.Binding,
		IngressId: circuit.IngressId,
		EgressId:  circuit.EgressId,
	}
	if err := network.setLinks(circuit2); err != nil {
		return nil, err
	}
	return circuit2, nil
}

func (network *Network) setLinks(circuit *Circuit) error {
	if len(circuit.Path) > 1 {
		for i := 0; i < len(circuit.Path)-1; i++ {
			if link, found := network.linkController.leastExpensiveLink(circuit.Path[i], circuit.Path[i+1]); found {
				circuit.Links = append(circuit.Links, link)
			} else {
				return errors.Errorf("no link from r/%v to r/%v", circuit.Path[i].Id, circuit.Path[i+1].Id)
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
			logrus.Infof("changed router [r/%s]", r.Id)
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
	logrus.Infof("changed link [l/%s]", l.Id.Token)
	if err := network.rerouteLink(l); err != nil {
		logrus.Errorf("unexpected error rerouting link (%s)", err)
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
	logrus.Infof("link [l/%s] changed", l.Id.Token)

	sessions := network.sessionController.all()
	for _, s := range sessions {
		if s.Circuit.usesLink(l) {
			logrus.Infof("session [s/%s] uses link [l/%s]", s.Id.Token, l.Id.Token)
			if err := network.rerouteSession(s); err != nil {
				logrus.Errorf("error rerouting session [s/%s], removing", s.Id.Token)
				if err := network.RemoveSession(s.Id, true); err != nil {
					logrus.Errorf("error removing session [s/%s] (%s)", s.Id.Token, err)
				}
			}
		}
	}

	return nil
}

func (network *Network) rerouteSession(s *Session) error {
	if s.Rerouting.CompareAndSwap(false, true) {
		defer s.Rerouting.Set(false)

		logrus.Warnf("rerouting [s/%s]", s.Id.Token)

		if cq, err := network.UpdateCircuit(s.Circuit); err == nil {
			s.Circuit = cq

			rms, err := cq.CreateRouteMessages(SmartRerouteAttempt, s.Id, s.Terminator.GetAddress())
			if err != nil {
				logrus.Errorf("error creating route messages (%s)", err)
				return err
			}

			for i := 0; i < len(cq.Path); i++ {
				if _, err := sendRoute(cq.Path[i], rms[i], network.options.RouteTimeout); err != nil {
					logrus.Errorf("error sending route to [r/%s] (%s)", cq.Path[i].Id, err)
				}
			}

			logrus.Infof("rerouted session [s/%s]", s.Id.Token)

			network.CircuitUpdated(s.Id, s.Circuit)

			return nil
		} else {
			return err
		}
	} else {
		logrus.Warnf("not rerouting [s/%s], already in progress", s.Id.Token)
		return nil
	}
}

func (network *Network) smartReroute(s *Session, cq *Circuit) error {
	if s.Rerouting.CompareAndSwap(false, true) {
		defer s.Rerouting.Set(false)

		s.Circuit = cq

		rms, err := cq.CreateRouteMessages(SmartRerouteAttempt, s.Id, s.Terminator.GetAddress())
		if err != nil {
			logrus.Errorf("error creating route messages (%s)", err)
			return err
		}

		for i := 0; i < len(cq.Path); i++ {
			if _, err := sendRoute(cq.Path[i], rms[i], network.options.RouteTimeout); err != nil {
				logrus.Errorf("error sending route to [r/%s] (%s)", cq.Path[i].Id, err)
			}
		}

		logrus.Debugf("rerouted session [s/%s]", s.Id.Token)

		network.CircuitUpdated(s.Id, s.Circuit)

		return nil

	} else {
		logrus.Warnf("not rerouting [s/%s], already in progress", s.Id.Token)
		return nil
	}
}

func (network *Network) AcceptMetrics(metrics *metrics_pb.MetricsMessage) {
	if metrics.SourceId == network.nodeId.Token {
		return // ignore metrics coming from the controller itself
	}

	log := pfxlog.Logger()

	router, err := network.Routers.Read(metrics.SourceId)
	if err != nil {
		log.Debugf("could not find router [r/%s] while processing metrics", metrics.SourceId)
		return
	}

	for _, link := range network.GetAllLinksForRouter(router.Id) {
		metricId := "link." + link.Id.Token + ".latency"
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
	pfxlog.Logger().Debugf("sending Create route message to [r/%s] for [s/%s]", r.Id, createMsg.SessionId)

	body, err := proto.Marshal(createMsg)
	if err != nil {
		return nil, err
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_RouteType), body)
	waitCh, err := r.Control.SendAndWait(msg)
	if err != nil {
		return nil, err
	}
	select {
	case msg := <-waitCh:
		if msg.ContentType == ctrl_msg.RouteResultType {
			_, success := msg.Headers[ctrl_msg.RouteResultSuccessHeader]
			if !success {
				message := ""
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

	case <-time.After(timeout):
		pfxlog.Logger().Errorf("timed out after %s waiting for response to route message from [r/%s] for [s/%s]", timeout, r.Id, createMsg.SessionId)
		return nil, errors.New("timeout")
	}
}

func sendUnroute(r *Router, sessionId *identity.TokenId, now bool) error {
	unroute := &ctrl_pb.Unroute{
		SessionId: sessionId.Token,
		Now:       now,
	}
	body, err := proto.Marshal(unroute)
	if err != nil {
		return err
	}
	unrouteMsg := channel2.NewMessage(int32(ctrl_pb.ContentType_UnrouteType), body)
	if err := r.Control.Send(unrouteMsg); err != nil {
		return err
	}
	return nil
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

type MultipleErrors []error

func (e MultipleErrors) Error() string {
	if len(e) == 0 {
		return "no errors occurred"
	}
	if len(e) == 1 {
		return e[0].Error()
	}
	buf := strings.Builder{}
	buf.WriteString("multiple errors occurred")
	for idx, err := range e {
		buf.WriteString(fmt.Sprintf(" %v: %v\n", idx, err))
	}
	return buf.String()
}

type InvalidSessionError struct {
	sessionId string
}

func (err InvalidSessionError) Error() string {
	return fmt.Sprintf("invalid session (%s)", err.sessionId)
}
