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
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/sequence"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Network struct {
	*Controllers
	nodeId                 *identity.TokenId
	options                *Options
	routerChanged          chan *Router
	linkController         *linkController
	linkChanged            chan *Link
	sessionController      *sessionController
	sequence               *sequence.Sequence
	eventDispatcher        event.Dispatcher
	traceController        trace.Controller
	routerPresenceHandlers []RouterPresenceHandler
	capabilities           []string
	shutdownChan           chan struct{}
	isShutdown             concurrenz.AtomicBoolean
	lock                   sync.Mutex
	strategyRegistry       xt.Registry
	lastSnapshot           time.Time
	metricsRegistry        metrics.Registry
	VersionProvider        common.VersionProvider
}

func NewNetwork(nodeId *identity.TokenId, options *Options, database boltz.Db, metricsCfg *metrics.Config, versionProvider common.VersionProvider) (*Network, error) {
	stores, err := db.InitStores(database)
	if err != nil {
		return nil, err
	}

	controllers := NewControllers(database, stores)
	eventDispatcher := event.NewDispatcher()

	network := &Network{
		Controllers:       controllers,
		nodeId:            nodeId,
		options:           options,
		routerChanged:     make(chan *Router),
		linkController:    newLinkController(),
		linkChanged:       make(chan *Link),
		sessionController: newSessionController(),
		sequence:          sequence.NewSequence(),
		eventDispatcher:   eventDispatcher,
		traceController:   trace.NewController(),
		shutdownChan:      make(chan struct{}),
		strategyRegistry:  xt.GlobalRegistry(),
		lastSnapshot:      time.Now().Add(-time.Hour),
		metricsRegistry:   metrics.NewRegistry(nodeId.Token, nil),
		VersionProvider:   versionProvider,
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
		dispatcher := metrics.NewDispatchWrapper(network.eventDispatcher.Dispatch)
		for {
			select {
			case <-timer.C:
				if msg := network.metricsRegistry.Poll(); msg != nil {
					dispatcher.AcceptMetrics(msg)
				}
			}
		}
	}()
}

func (network *Network) GetAppId() *identity.TokenId {
	return network.nodeId
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

func (network *Network) GetSession(sessionId *identity.TokenId) (*session, bool) {
	return network.sessionController.get(sessionId)
}

func (network *Network) GetAllSessions() []*session {
	return network.sessionController.all()
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

func (network *Network) CreateSession(srcR *Router, clientId *identity.TokenId, service string) (*session, error) {
	log := pfxlog.Logger()

	// 1: Allocate Session Identifier
	sessionIdHash, err := network.sequence.NextHash()
	if err != nil {
		return nil, err
	}
	sessionId := &identity.TokenId{Token: sessionIdHash}

	targetIdentity, serviceId := parseIdentityAndService(service)

	retryCount := 0
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
		rms, err := circuit.CreateRouteMessages(sessionId, terminator.GetAddress())
		if err != nil {
			return nil, err
		}

		// 5: Route Egress
		rms[len(rms)-1].Egress.PeerData = clientId.Data
		peerData, err := sendRoute(circuit.Path[len(circuit.Path)-1], rms[len(rms)-1])
		if err != nil {
			strategy.NotifyEvent(xt.NewDialFailedEvent(terminator))
			retryCount++
			if retryCount > 3 {
				return nil, err
			} else {
				continue
			}
		} else {
			strategy.NotifyEvent(xt.NewDialSucceeded(terminator))
		}

		// 6: Create Intermediate Routes
		for i := 0; i < len(circuit.Path)-1; i++ {
			_, err = sendRoute(circuit.Path[i], rms[i])
			if err != nil {
				return nil, err
			}
		}

		// 7: Create Session Object
		ss := &session{
			Id:         sessionId,
			ClientId:   clientId,
			Service:    svc,
			Circuit:    circuit,
			Terminator: terminator,
			PeerData:   peerData,
		}
		network.sessionController.add(ss)
		network.SessionCreated(ss.Id, ss.ClientId, ss.Service.Id, ss.Circuit)

		log.Debugf("created session [s/%s] ==> %s", sessionId.Token, ss.Circuit)
		return ss, nil
	}
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
				err := errors.Errorf("invalid terminating router %v on terminator %v", terminator.GetRouterId(), terminator.GetId())
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
		return nil, nil, nil, errors.Errorf("service %v has no Terminators", svc.Id)
	}

	if len(weightedTerminators) == 0 && len(errList) == 0 {
		return nil, nil, nil, errors.Errorf("service %v has no Terminators for identity %v", svc.Id, identity)
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
	log := pfxlog.Logger()
	defer log.Error("exited")
	log.Info("started")

	for {
		select {
		case r := <-network.routerChanged:
			log.Infof("changed router [r/%s]", r.Id)
			network.assemble()
			network.clean()

		case l := <-network.linkChanged:
			log.Infof("changed link [l/%s]", l.Id.Token)
			if err := network.rerouteLink(l); err != nil {
				log.Errorf("unexpected error rerouting link (%s)", err)
			}

		case <-time.After(time.Duration(network.options.CycleSeconds) * time.Second):
			network.assemble()
			network.clean()
			network.smart()
		case _, ok := <-network.shutdownChan:
			if !ok {
				return
			}
		}
	}
}

func (network *Network) Shutdown() {
	if network.isShutdown.CompareAndSwap(false, true) {
		close(network.shutdownChan)
		events.RemoveMetricsEventHandler(network)
	}
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
	log := pfxlog.Logger()
	log.Infof("link [l/%s] changed", l.Id.Token)

	sessions := network.sessionController.all()
	for _, s := range sessions {
		if s.Circuit.usesLink(l) {
			log.Infof("session [s/%s] uses link [l/%s]", s.Id.Token, l.Id.Token)
			if err := network.rerouteSession(s); err != nil {
				log.Errorf("error rerouting session [s/%s], removing", s.Id.Token)
				if err := network.RemoveSession(s.Id, true); err != nil {
					log.Errorf("error removing session [s/%s] (%s)", s.Id.Token, err)
				}
			}
		}
	}

	return nil
}

func (network *Network) rerouteSession(s *session) error {
	log := pfxlog.Logger()
	log.Warnf("rerouting [s/%s]", s.Id.Token)

	if cq, err := network.UpdateCircuit(s.Circuit); err == nil {
		s.Circuit = cq

		rms, err := cq.CreateRouteMessages(s.Id, s.Terminator.GetAddress())
		if err != nil {
			log.Errorf("error creating route messages (%s)", err)
			return err
		}

		for i := 0; i < len(cq.Path); i++ {
			if _, err := sendRoute(cq.Path[i], rms[i]); err != nil {
				log.Errorf("error sending route to [r/%s] (%s)", cq.Path[i].Id, err)
			}
		}

		log.Infof("rerouted session [s/%s]", s.Id.Token)

		network.CircuitUpdated(s.Id, s.Circuit)

		return nil
	} else {
		return err
	}
}

func (network *Network) smartReroute(s *session, cq *Circuit) error {
	log := pfxlog.Logger()

	s.Circuit = cq

	rms, err := cq.CreateRouteMessages(s.Id, s.Terminator.GetAddress())
	if err != nil {
		log.Errorf("error creating route messages (%s)", err)
		return err
	}

	for i := 0; i < len(cq.Path); i++ {
		if _, err := sendRoute(cq.Path[i], rms[i]); err != nil {
			log.Errorf("error sending route to [r/%s] (%s)", cq.Path[i].Id, err)
		}
	}

	log.Debugf("rerouted session [s/%s]", s.Id.Token)

	network.CircuitUpdated(s.Id, s.Circuit)

	return nil
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

func sendRoute(r *Router, createMsg *ctrl_pb.Route) (xt.PeerData, error) {
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
		if msg.ContentType == channel2.ContentTypeResultType {
			result := channel2.UnmarshalResult(msg)

			if !result.Success {
				return nil, errors.New(result.Message)
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

	case <-time.After(10 * time.Second):
		pfxlog.Logger().Errorf("timed out waiting for response to route message from [r/%s] for [s/%s]", r.Id, createMsg.SessionId)
		return nil, errors.New("timeout")
	}
}

func sendUnroute(r *Router, sessionId *identity.TokenId, now bool) error {
	remove := &ctrl_pb.Unroute{
		SessionId: sessionId.Token,
		Now:       now,
	}
	body, err := proto.Marshal(remove)
	if err != nil {
		return err
	}
	removeMsg := channel2.NewMessage(int32(ctrl_pb.ContentType_UnrouteType), body)
	if err := r.Control.Send(removeMsg); err != nil {
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
