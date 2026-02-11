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

package handler_ctrl

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/inspect"
	fabricMetrics "github.com/openziti/ziti/v2/common/metrics"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/sirupsen/logrus"
)

// CtrlDialer manages outbound ctrl channel connections to routers that advertise ctrl channel
// listener addresses. It runs a single-threaded event loop that tracks per-router dial state
// and dispatches dial attempts to a worker pool with exponential backoff on failure.
type CtrlDialer struct {
	config          *config.CtrlDialerConfig
	network         *network.Network
	ctrlAccepter    *CtrlAccepter
	ctrlId          *identity.TokenId
	headers         map[int32][]byte
	closeNotify     <-chan struct{}
	metricsRegistry metrics.Registry

	events     chan dialEvent
	states     map[string]*routerDialState
	retryQueue dialStateHeap
	dialPool   goroutines.Pool
}

// NewCtrlDialer creates a new CtrlDialer. Call Run to start the event loop.
func NewCtrlDialer(
	config *config.CtrlDialerConfig,
	network *network.Network,
	ctrlAccepter *CtrlAccepter,
	ctrlId *identity.TokenId,
	headers map[int32][]byte,
	closeNotify <-chan struct{},
	metricsRegistry metrics.Registry,
) *CtrlDialer {
	return &CtrlDialer{
		config:          config,
		network:         network,
		ctrlAccepter:    ctrlAccepter,
		ctrlId:          ctrlId,
		headers:         headers,
		closeNotify:     closeNotify,
		metricsRegistry: metricsRegistry,
		events:          make(chan dialEvent, 64),
		states:          make(map[string]*routerDialState),
	}
}

// Run starts the dialer event loop. It blocks until closeNotify is closed.
func (self *CtrlDialer) Run() {
	log := pfxlog.Logger().WithField("component", "ctrlDialer")
	log.WithField("dialDelay", self.config.DialDelay).
		WithField("groups", self.config.Groups).
		Info("starting ctrl channel dialer")

	poolConfig := goroutines.PoolConfig{
		QueueSize:   self.config.QueueSize,
		MinWorkers:  0,
		MaxWorkers:  self.config.MaxWorkers,
		IdleTime:    30 * time.Second,
		CloseNotify: self.closeNotify,
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().WithField("component", "ctrlDialer").
				Errorf("panic in dial worker: %v", err)
		},
	}
	fabricMetrics.ConfigureGoroutinesPoolMetrics(&poolConfig, self.metricsRegistry, "ctrl_channel.dialer")
	pool, err := goroutines.NewPool(poolConfig)
	if err != nil {
		log.WithError(err).Error("failed to create dial pool")
		return
	}
	self.dialPool = pool

	if self.config.DialDelay > 0 {
		select {
		case <-time.After(self.config.DialDelay):
		case <-self.closeNotify:
			return
		}
	}

	self.scan()

	fullScanTicker := time.NewTicker(time.Hour)
	defer fullScanTicker.Stop()

	queueCheckTicker := time.NewTicker(5 * time.Second)
	defer queueCheckTicker.Stop()

	for {
		select {
		case evt := <-self.events:
			evt.handle(self)
		case <-queueCheckTicker.C:
			self.evaluateRetryQueue()
		case <-fullScanTicker.C:
			self.scan()
		case <-self.closeNotify:
			log.Info("stopping ctrl channel dialer")
			return
		}
	}
}

// RouterConnected notifies the dialer that a router has connected (e.g. via inbound accept).
func (self *CtrlDialer) RouterConnected(r *model.Router) {
	self.queueEvent(&routerConnectedEvent{routerId: r.Id})
}

// RouterDisconnected notifies the dialer that a router's ctrl channel has been lost.
func (self *CtrlDialer) RouterDisconnected(r *model.Router) {
	self.queueEvent(&routerDisconnectedEvent{routerId: r.Id, router: r})
}

// RouterUpdated notifies the dialer that a router's configuration has changed.
func (self *CtrlDialer) RouterUpdated(id string) {
	self.queueEvent(&routerUpdatedEvent{routerId: id})
}

// RouterCreated notifies the dialer that a new router has been created.
func (self *CtrlDialer) RouterCreated(id string) {
	self.queueEvent(&routerUpdatedEvent{routerId: id})
}

// RouterDeleted notifies the dialer that a router has been deleted.
func (self *CtrlDialer) RouterDeleted(id string) {
	self.queueEvent(&routerDeletedEvent{routerId: id})
}

func (self *CtrlDialer) queueEvent(evt dialEvent) {
	select {
	case <-self.closeNotify:
	case self.events <- evt:
	}
}

// Validate cross-references dial states against the router store and returns per-router diagnostic details.
func (self *CtrlDialer) Validate() ([]*mgmt_pb.ControllerDialerDetails, error) {
	// Get a snapshot of the dialer states from the event loop
	evt := &validateDialStatesEvent{
		result: make(chan map[string]*validateDialStateSnapshot, 1),
	}

	select {
	case self.events <- evt:
	case <-self.closeNotify:
		return nil, nil
	}

	var dialStates map[string]*validateDialStateSnapshot
	select {
	case dialStates = <-evt.result:
	case <-time.After(time.Second):
		return nil, fmt.Errorf("timeout waiting for dialer state")
	case <-self.closeNotify:
		return nil, nil
	}

	// List all routers
	routers, err := self.network.Router.BaseList("limit none")
	if err != nil {
		return nil, fmt.Errorf("error listing routers: %w", err)
	}

	// Track which dial states we've accounted for
	accountedStates := map[string]struct{}{}

	var results []*mgmt_pb.ControllerDialerDetails
	for _, router := range routers.Entities {
		if router.Disabled {
			// If a disabled router still has a dial state, that's stale
			if state, inDialStates := dialStates[router.Id]; inDialStates {
				accountedStates[router.Id] = struct{}{}
				results = append(results, &mgmt_pb.ControllerDialerDetails{
					ComponentId:   router.Id,
					ComponentName: router.Name,
					Errors:        []string{fmt.Sprintf("router is disabled but has stale dial state (%s)", state.status)},
				})
			}
			continue
		}

		hasMatchingEndpoints := false
		for _, groups := range router.CtrlChanListeners {
			if self.groupsMatch(groups) {
				hasMatchingEndpoints = true
				break
			}
		}
		if !hasMatchingEndpoints {
			// If a router with no matching endpoints still has a dial state, that's stale
			if state, inDialStates := dialStates[router.Id]; inDialStates {
				accountedStates[router.Id] = struct{}{}
				results = append(results, &mgmt_pb.ControllerDialerDetails{
					ComponentId:   router.Id,
					ComponentName: router.Name,
					Errors:        []string{fmt.Sprintf("router has no matching ctrl channel listeners but has stale dial state (%s)", state.status)},
				})
			}
			continue
		}

		accountedStates[router.Id] = struct{}{}

		detail := &mgmt_pb.ControllerDialerDetails{
			ComponentId:   router.Id,
			ComponentName: router.Name,
		}

		connected := self.network.GetConnectedRouter(router.Id) != nil
		state, inDialStates := dialStates[router.Id]

		if connected && !inDialStates {
			detail.ValidateSuccess = true
		} else if connected && inDialStates {
			if state.status == statusNeedsDial || state.status == statusDialing {
				detail.Errors = append(detail.Errors, fmt.Sprintf("connected but dialer state is %s", state.status))
			} else {
				// statusConnected in dial states is normal (waiting for FastFailureWindow)
				detail.ValidateSuccess = true
			}
		} else if !connected && inDialStates {
			if state.status == statusNeedsDial || state.status == statusDialing {
				detail.Errors = append(detail.Errors, fmt.Sprintf("not connected, dial state is %s", state.status))
			} else {
				detail.Errors = append(detail.Errors, "not connected, dial state is Connected (stale)")
			}
		} else {
			// not connected, not in dial states
			detail.Errors = append(detail.Errors, "not connected and not being dialed")
		}

		detail.ValidateSuccess = len(detail.Errors) == 0
		results = append(results, detail)
	}

	// Check for dial states referencing routers that no longer exist or weren't covered above
	for routerId, state := range dialStates {
		if _, accounted := accountedStates[routerId]; !accounted {
			results = append(results, &mgmt_pb.ControllerDialerDetails{
				ComponentId: routerId,
				Errors:      []string{fmt.Sprintf("dial state (%s) exists for unknown or deleted router", state.status)},
			})
		}
	}

	return results, nil
}

// Inspect handles inspection requests for the ctrl dialer, returning current dial state as JSON.
func (self *CtrlDialer) Inspect(name string) (bool, *string, error) {
	if !strings.EqualFold(name, inspect.CtrlDialerKey) {
		return false, nil, nil
	}

	evt := &inspectDialStatesEvent{
		result: make(chan *inspect.CtrlDialerInspectResult, 1),
	}

	select {
	case self.events <- evt:
	case <-self.closeNotify:
		return false, nil, nil
	}

	select {
	case result := <-evt.result:
		js, err := json.Marshal(result)
		if err != nil {
			return true, nil, err
		}
		val := string(js)
		return true, &val, nil
	case <-time.After(time.Second):
		return true, nil, nil
	case <-self.closeNotify:
		return false, nil, nil
	}
}

func (self *CtrlDialer) scan() {
	log := pfxlog.Logger().WithField("component", "ctrlDialer")

	routers, err := self.network.Router.BaseList("limit none")
	if err != nil {
		log.WithError(err).Error("error listing routers for ctrl dialer scan")
		return
	}

	newCount := 0
	spreadInterval := 50 * time.Millisecond

	for _, router := range routers.Entities {
		addresses := self.getRouterIdEndpointsNeedingDial(router.Id, router)
		if len(addresses) == 0 {
			continue
		}

		if state, exists := self.states[router.Id]; exists {
			if state.status == statusConnected || state.status == statusDialing {
				continue
			}
			// already in NeedsDial, leave it on the heap
			continue
		}

		state := &routerDialState{
			routerId:  router.Id,
			addresses: addresses,
			nextDial:  time.Now().Add(time.Duration(newCount) * spreadInterval),
		}
		self.states[router.Id] = state
		heap.Push(&self.retryQueue, state)
		newCount++
	}

	if newCount > 0 {
		log.WithField("newRouters", newCount).Debug("scan found routers needing dial")
	}
}

func (self *CtrlDialer) evaluateRetryQueue() {
	now := time.Now()
	for self.retryQueue.Len() > 0 {
		state := self.retryQueue[0]
		if state.nextDial.After(now) {
			break
		}
		heap.Pop(&self.retryQueue)
		self.evaluateDialState(state)
	}
}

func (self *CtrlDialer) evaluateDialState(state *routerDialState) {
	log := pfxlog.Logger().WithField("routerId", state.routerId)

	// if connected, check if it's still connected (fast failure detection)
	if state.status == statusConnected {
		if self.network.GetConnectedRouter(state.routerId) != nil {
			// still connected and survived past FastFailureWindow — reset backoff and stop tracking
			state.retryDelay = 0
			state.dialAttempts = 0
			delete(self.states, state.routerId)
			return
		}
		// was marked connected but now disconnected — treat as connection lost
		state.connectionLost(self.config)
		heap.Push(&self.retryQueue, state)
		return
	}

	// check if router still needs a dial
	addresses := self.getRouterIdEndpointsNeedingDial(state.routerId, nil)
	if len(addresses) == 0 {
		delete(self.states, state.routerId)
		return
	}
	state.addresses = addresses
	if state.addrIndex >= len(state.addresses) {
		state.addrIndex = state.addrIndex % len(state.addresses)
	}

	if !state.dialActive.CompareAndSwap(false, true) {
		return // already being dialed
	}

	state.status = statusDialing

	err := self.dialPool.QueueOrError(func() {
		self.doDial(state)
	})

	if err != nil {
		state.dialActive.Store(false)
		log.WithError(err).Warn("unable to queue dial, pool full")
		state.dialFailed(self.config)
		heap.Push(&self.retryQueue, state)
	}
}

func (self *CtrlDialer) doDial(state *routerDialState) {
	defer state.dialActive.Store(false)

	address := state.currentAddress()

	log := pfxlog.Logger().WithField("component", "ctrlDialer").
		WithField("routerId", state.routerId).
		WithField("address", address)

	addr, err := transport.ParseAddress(address)
	if err != nil {
		log.WithError(err).Error("error parsing ctrl chan listener address")
		self.queueEvent(&dialResultEvent{routerId: state.routerId, err: err})
		return
	}

	log.Info("dialing router")
	dialErr := self.dial(state.routerId, addr, log)
	self.queueEvent(&dialResultEvent{routerId: state.routerId, err: dialErr})
}

func (self *CtrlDialer) getRouterIdEndpointsNeedingDial(routerId string, router *model.Router) []string {
	log := pfxlog.Logger().WithField("routerId", routerId)
	if self.network.GetConnectedRouter(routerId) != nil {
		return nil
	}

	if router == nil {
		var err error
		router, err = self.network.Router.BaseLoad(routerId)
		if err != nil {
			log.WithError(err).Error("error loading router")
			return nil
		}
	}

	if router.Disabled {
		return nil
	}

	var results []string
	for address, groups := range router.CtrlChanListeners {
		if self.groupsMatch(groups) {
			results = append(results, address)
		}
	}

	return results
}

func (self *CtrlDialer) groupsMatch(routerGroups []string) bool {
	if len(routerGroups) == 0 {
		routerGroups = []string{"default"}
	}
	for _, rg := range routerGroups {
		for _, cg := range self.config.Groups {
			if rg == cg {
				return true
			}
		}
	}
	return false
}

func (self *CtrlDialer) dial(routerId string, addr transport.Address, log *logrus.Entry) error {
	dialer := channel.NewClassicDialer(channel.DialerConfig{
		Identity: self.ctrlId,
		Endpoint: addr,
		Headers:  self.headers,
		TransportConfig: transport.Configuration{
			"protocol": "ziti-ctrl",
		},
	})

	firstDialHeaders := make(channel.Headers, 3)
	firstDialHeaders.PutBoolHeader(channel.IsGroupedHeader, true)
	firstDialHeaders.PutStringHeader(channel.TypeHeader, ctrlchan.ChannelTypeDefault)
	firstDialHeaders.PutBoolHeader(channel.IsFirstGroupConnection, true)

	underlay, err := dialer.CreateWithHeaders(self.ctrlAccepter.options.ConnectTimeout, firstDialHeaders)
	if err != nil {
		return err
	}

	listenerCtrlChan := ctrlchan.NewListenerCtrlChannel()

	multiConfig := &channel.MultiChannelConfig{
		LogicalName:     "ctrl/" + underlay.Id(),
		Options:         self.ctrlAccepter.options,
		UnderlayHandler: listenerCtrlChan,
		BindHandler: channel.BindHandlerF(func(binding channel.Binding) error {
			binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
				time.AfterFunc(time.Second, func() {
					self.queueEvent(&routerDisconnectedEvent{routerId: routerId})
				})
			}))
			return self.ctrlAccepter.Bind(binding)
		}),
		Underlay: underlay,
	}

	if _, err = channel.NewMultiChannel(multiConfig); err != nil {
		if closeErr := underlay.Close(); closeErr != nil {
			log.WithError(closeErr).Error("error closing underlay after multi channel creation failure")
		}
		return err
	}

	return nil
}
