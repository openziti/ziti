/*
	Copyright 2019 NetFoundry, Inc.

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
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-fabric/metrics"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-fabric/trace"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/concurrenz"
	"github.com/netfoundry/ziti-foundation/util/sequence"
	"strings"
	"sync"
	"time"
)

type Network struct {
	nodeId                     *identity.TokenId
	db                         *db.Db
	options                    *Options
	routerController           *routerController
	routerStore                RouterStore
	routerChanged              chan *Router
	linkController             *linkController
	linkChanged                chan *Link
	serviceController          *serviceController
	sessionController          *sessionController
	sequence                   *sequence.Sequence
	metricsEventController     metrics.EventController
	sessionLifeCycleController SessionLifeCycleController
	traceEventController       trace.EventController
	traceController            trace.Controller
	routerPresenceHandlers     []RouterPresenceHandler
	capabilities               []string
	shutdownChan               chan struct{}
	isShutdown                 concurrenz.AtomicBoolean
	lock                       sync.Mutex
}

func NewNetwork(nodeId *identity.TokenId, options *Options, db *db.Db, metricsCfg *metrics.Config) (*Network, error) {
	serviceController, err := newServiceController(db)
	if err != nil {
		return nil, err
	}
	network := &Network{
		nodeId:                     nodeId,
		db:                         db,
		options:                    options,
		routerController:           newRouterController(),
		routerStore:                NewRouterStore(db),
		routerChanged:              make(chan *Router),
		linkController:             newLinkController(),
		linkChanged:                make(chan *Link),
		serviceController:          serviceController,
		sessionController:          newSessionController(),
		sequence:                   sequence.NewSequence(),
		metricsEventController:     metrics.NewEventController(metricsCfg),
		sessionLifeCycleController: NewSessionLifeCyleController(),
		traceEventController:       trace.NewEventController(),
		traceController:            trace.NewController(),
		shutdownChan:               make(chan struct{}),
	}
	network.metricsEventController.AddHandler(network)
	network.AddCapability("ziti.fabric")
	network.showOptions()
	return network, nil
}

func (network *Network) GetAppId() *identity.TokenId {
	return network.nodeId
}

func (network *Network) GetDb() boltz.Db {
	return network.db
}

func (network *Network) GetRouterStore() RouterStore {
	return network.routerStore
}

func (network *Network) CreateRouter(router *Router) error {
	return network.routerStore.create(router)
}

func (network *Network) GetConnectedRouter(routerId string) (*Router, bool) {
	return network.routerController.get(routerId)
}

func (network *Network) GetRouter(routerId string) (*Router, error) {
	return network.routerStore.loadOneById(routerId)
}

func (network *Network) AllRouters() ([]*Router, error) {
	return network.routerStore.all()
}

func (network *Network) AllConnectedRouters() []*Router {
	return network.routerController.all()
}

func (network *Network) RemoveRouter(router *Router) error {
	return network.routerStore.remove(router.Id)
}

func (network *Network) GetServiceStore() ServiceStore {
	return network.serviceController.store
}

func (network *Network) CreateService(service *Service) error {
	return network.serviceController.create(service)
}

func (network *Network) GetService(serviceId string) (*Service, bool) {
	return network.serviceController.get(serviceId)
}

func (network *Network) AllServices() ([]*Service, error) {
	return network.serviceController.all()
}

func (network *Network) RemoveService(id string) error {
	return network.serviceController.remove(id)
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

func (network *Network) GetMetricsEventController() metrics.EventController {
	return network.metricsEventController
}

func (network *Network) GetSessionLifeCycleController() SessionLifeCycleController {
	return network.sessionLifeCycleController
}

func (network *Network) GetTraceEventController() trace.EventController {
	return network.traceEventController
}

func (network *Network) GetTraceController() trace.Controller {
	return network.traceController
}

func (network *Network) RouterChanged(r *Router) {
	network.routerChanged <- r
}

func (network *Network) ConnectedRouter(id string) bool {
	return network.routerController.has(id)
}

func (network *Network) KnownRouter(id string) (*Router, error) {
	r, err := network.routerStore.loadOneById(id)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (network *Network) ConnectRouter(r *Router) {
	network.routerController.add(r)
	network.routerChanged <- r

	for _, h := range network.routerPresenceHandlers {
		h.RouterConnected(r)
	}
}

func (network *Network) DisconnectRouter(r *Router) {
	// 1: remove Links for Router
	for _, l := range network.linkController.allLinksForRouter(r.Id) {
		network.linkController.remove(l)
		network.LinkChanged(l)
	}
	// 2: remove Router
	network.routerController.remove(r)
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

func (network *Network) BindService(srcR *Router, token string, serviceId string) error {
	// 1: Find Service
	if svc, found := network.serviceController.get(serviceId); found {
		svc.Egress = srcR.Id
		svc.EndpointAddress = "hosted:" + token

		return network.serviceController.update(svc)
	}
	return errors.New("invalid service")
}

func (network *Network) UnbindService(srcR *Router, token string, serviceId string) error {
	// 1: Find Service
	if svc, found := network.serviceController.get(serviceId); found {
		svc.Egress = network.nodeId.Token // TODO: can't set this to null, so setting it to the controller ID
		svc.EndpointAddress = "hosted:unbound"

		return network.serviceController.update(svc)
	}
	return errors.New("invalid service")
}

func (network *Network) CreateSession(srcR *Router, clientId *identity.TokenId, serviceId string) (*session, error) {
	log := pfxlog.Logger()

	// 1: Find Service
	if svc, found := network.serviceController.get(serviceId); found {
		// 2: Allocate Session Identifier
		sessionIdHash, err := network.sequence.NextHash()
		if err != nil {
			return nil, err
		}
		sessionId := &identity.TokenId{Token: sessionIdHash}

		// 3: Get Egress Router
		if er, found := network.routerController.get(svc.Egress); found {
			// 4: Create Circuit
			circuit, err := network.CreateCircuit(srcR, er)
			if err != nil {
				return nil, err
			}
			circuit.Binding = "transport"
			if svc.Binding != "" {
				circuit.Binding = svc.Binding
			} else if strings.HasPrefix(svc.EndpointAddress, "hosted") {
				circuit.Binding = "edge"
			} else if strings.HasPrefix(svc.EndpointAddress, "udp") {
				circuit.Binding = "udp"
			}

			// 4a: Create Route Messages
			rms, err := circuit.CreateRouteMessages(sessionId, svc.EndpointAddress)
			if err != nil {
				return nil, err
			}

			// 5: Route Egress
			err = sendRoute(circuit.Path[len(circuit.Path)-1], rms[len(rms)-1])
			if err != nil {
				return nil, err
			}

			// 6: Create Intermediate Routes
			for i := 0; i < len(circuit.Path)-1; i++ {
				err = sendRoute(circuit.Path[i], rms[i])
				if err != nil {
					return nil, err
				}
			}

			// 7: Create Session Object
			ss := &session{
				Id:       sessionId,
				ClientId: clientId,
				Service:  svc,
				Circuit:  circuit,
			}
			network.sessionController.add(ss)
			network.sessionLifeCycleController.SessionCreated(ss.Id, ss.ClientId, ss.Service.Id, ss.Circuit)

			log.Infof("created session [s/%s] ==> %s", sessionId.Token, ss.Circuit)

			return ss, nil

		}
		return nil, errors.New("missing egress")
	}
	return nil, errors.New("invalid service")
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
		network.sessionLifeCycleController.SessionDeleted(sessionId)

		log.Infof("removed session [s/%s]", ss.Id.Token)

		return nil
	}
	return fmt.Errorf("invalid session (%s)", sessionId.Token)
}

func (network *Network) StartSessionEgress(sessionId *identity.TokenId) error {
	log := pfxlog.Logger()

	if ss, found := network.sessionController.get(sessionId); found {
		terminatingRouter := ss.Circuit.Path[len(ss.Circuit.Path)-1]
		log.Infof("started session egress [s/%s]", ss.Id.Token)
		return sendStartXgress(terminatingRouter, sessionId)
	}
	return fmt.Errorf("invalid session (%s)", sessionId.Token)
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

func (network *Network) UpdateCircuit(circuit *Circuit) (*Circuit, error) {
	srcR := circuit.Path[0]
	dstR := circuit.Path[len(circuit.Path)-1]
	path, err := network.shortestPath(srcR, dstR)
	if err != nil {
		return nil, err
	}

	circuit2 := &Circuit{
		Path:      path,
		Binding:   circuit.Binding,
		IngressId: circuit.IngressId,
		EgressId:  circuit.EgressId,
	}

	if len(path) > 1 {
		for i := 0; i < len(path)-1; i++ {
			if link, found := network.linkController.leastExpensiveLink(path[i], path[i+1]); found {
				circuit2.Links = append(circuit2.Links, link)
			}
		}
	}

	return circuit2, nil
}

func (network *Network) AddRouterPresenceHandler(h RouterPresenceHandler) {
	network.routerPresenceHandlers = append(network.routerPresenceHandlers, h)
}

func (network *Network) Debug() string {
	return "oh, wow"
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

		rms, err := cq.CreateRouteMessages(s.Id, s.Service.EndpointAddress)
		if err != nil {
			log.Errorf("error creating route messages (%s)", err)
			return err
		}

		for i := 0; i < len(cq.Path); i++ {
			if err := sendRoute(cq.Path[i], rms[i]); err != nil {
				log.Errorf("error sending route to [r/%s] (%s)", cq.Path[i].Id, err)
			}
		}

		log.Infof("rerouted session [s/%s]", s.Id.Token)

		network.sessionLifeCycleController.CircuitUpdated(s.Id, s.Circuit)

		return nil
	} else {
		return err
	}
}

func (network *Network) smartReroute(s *session, cq *Circuit) error {
	log := pfxlog.Logger()

	s.Circuit = cq

	rms, err := cq.CreateRouteMessages(s.Id, s.Service.EndpointAddress)
	if err != nil {
		log.Errorf("error creating route messages (%s)", err)
		return err
	}

	for i := 0; i < len(cq.Path); i++ {
		if err := sendRoute(cq.Path[i], rms[i]); err != nil {
			log.Errorf("error sending route to [r/%s] (%s)", cq.Path[i].Id, err)
		}
	}

	log.Debugf("rerouted session [s/%s]", s.Id.Token)

	network.sessionLifeCycleController.CircuitUpdated(s.Id, s.Circuit)

	return nil
}

func (network *Network) AcceptMetrics(metrics *ctrl_pb.MetricsMessage) {
	log := pfxlog.Logger()

	router, err := network.routerStore.loadOneById(metrics.SourceId)
	if err != nil {
		log.Warnf("could not find router [r/%s] while processing metrics", metrics.SourceId)
		return
	}

	for _, link := range network.GetAllLinksForRouter(router.Id) {
		metricId := "link." + link.Id.Token + ".latency"
		if latency, ok := metrics.Histograms[metricId]; ok {
			if link.Src.Id == router.Id {
				link.SrcLatency = int64(latency.Mean)
			} else if link.Dst.Id == router.Id {
				link.DstLatency = int64(latency.Mean)
			} else {
				log.Warnf("link not for router (wtf?)")
			}
		}
	}
}

func sendRoute(r *Router, createMsg *ctrl_pb.Route) error {
	pfxlog.Logger().Debugf("sending create route message to [r/%s] for [s/%s]", r.Id, createMsg.SessionId)

	body, err := proto.Marshal(createMsg)
	if err != nil {
		return err
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_RouteType), body)
	waitCh, err := r.Control.SendAndWait(msg)
	if err != nil {
		return err
	}
	select {
	case msg := <-waitCh:
		if msg.ContentType == channel2.ContentTypeResultType {
			result := channel2.UnmarshalResult(msg)

			if !result.Success {
				return errors.New(result.Message)
			}
			return nil
		}
		return fmt.Errorf("unexpected response type %v received in reply to route request", msg.ContentType)

	case <-time.After(10 * time.Second):
		pfxlog.Logger().Errorf("timed out waiting for response to route message from [r/%s] for [s/%s]", r.Id, createMsg.SessionId)
		return errors.New("timeout")
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

func sendStartXgress(r *Router, sessionId *identity.TokenId) error {
	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_StartXgressType), []byte(sessionId.Token))
	if err := r.Control.Send(msg); err != nil {
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
	return network.serviceController
}

type Cache interface {
	RemoveFromCache(id string)
}