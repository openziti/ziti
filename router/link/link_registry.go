/*
	(c) Copyright NetFoundry Inc.

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

package link

import (
	"container/heap"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/common/capabilities"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xlink"
	"github.com/sirupsen/logrus"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Env interface {
	GetRouterId() *identity.TokenId
	GetNetworkControllers() env.NetworkControllers
	GetXlinkDialers() []xlink.Dialer
	GetCloseNotify() <-chan struct{}
	GetLinkDialerPool() goroutines.Pool
	GetRateLimiterPool() goroutines.Pool
	GetMetricsRegistry() metrics.UsageRegistry
}

func NewLinkRegistry(routerEnv Env) xlink.Registry {
	result := &linkRegistryImpl{
		linkMap:        map[string]xlink.Xlink{},
		linkByIdMap:    map[string]xlink.Xlink{},
		ctrls:          routerEnv.GetNetworkControllers(),
		events:         make(chan event, 16),
		env:            routerEnv,
		destinations:   map[string]*linkDest{},
		linkStateQueue: &linkStateHeap{},
		triggerNotifyC: make(chan struct{}, 1),
	}

	go result.run()
	go result.runGcLinkMetricsLoop()

	return result
}

type linkRegistryImpl struct {
	linkMapLocks sync.RWMutex
	linkMap      map[string]xlink.Xlink
	linkByIdMap  map[string]xlink.Xlink
	sync.Mutex
	ctrls env.NetworkControllers

	env              Env
	destinations     map[string]*linkDest
	linkStateQueue   *linkStateHeap
	events           chan event
	triggerNotifyC   chan struct{}
	notifyInProgress atomic.Bool
}

func (self *linkRegistryImpl) runGcLinkMetricsLoop() {
	var lastRunResults map[string]metrics.Metric

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lastRunResults = self.gcLinkMetrics(lastRunResults)
		case <-self.env.GetCloseNotify():
			return
		}
	}
}

func (self *linkRegistryImpl) gcLinkMetrics(lastRunResults map[string]metrics.Metric) map[string]metrics.Metric {
	for linkId, metric := range lastRunResults {
		// If the metrics we found last time are still tied to a non-existent link, dispose of them
		if !self.IsKnownLinkId(linkId) {
			metric.Dispose()
		}
	}
	return self.getOrphanedLinkMetrics()
}

func (self *linkRegistryImpl) getOrphanedLinkMetrics() map[string]metrics.Metric {
	result := map[string]metrics.Metric{}
	self.env.GetMetricsRegistry().EachMetric(func(name string, metric metrics.Metric) {
		if strings.HasPrefix(name, "link.") {
			base := strings.TrimPrefix(name, "link.")
			var linkId string
			if strings.ContainsRune(base, ':') {
				parts := strings.Split(base, ":")
				if len(parts) != 2 {
					return
				}
				linkId = parts[1]
			} else {
				linkId = strings.Split(base, ".")[0]
			}

			if !self.IsKnownLinkId(linkId) {
				result[name] = metric
			}
		}
	})

	return result
}

func (self *linkRegistryImpl) IsKnownLinkId(linkId string) bool {
	if _, found := self.GetLinkById(linkId); found {
		return true
	}

	evt := &scanForLinkIdEvent{
		linkId:  linkId,
		resultC: make(chan bool, 1),
	}
	self.queueEvent(evt)

	t := time.NewTicker(time.Second)
	defer t.Stop()

	// if we can't tell, defer to link known so we don't clear anything
	select {
	case result := <-evt.resultC:
		return result
	case <-self.env.GetCloseNotify():
		return true
	case <-t.C:
		return true
	}
}

func (self *linkRegistryImpl) GetLink(linkKey string) (xlink.Xlink, bool) {
	self.linkMapLocks.RLock()
	defer self.linkMapLocks.RUnlock()

	val, found := self.linkMap[linkKey]
	return val, found
}

func (self *linkRegistryImpl) GetLinkById(linkId string) (xlink.Xlink, bool) {
	self.linkMapLocks.RLock()
	defer self.linkMapLocks.RUnlock()

	link, found := self.linkByIdMap[linkId]
	return link, found
}

func (self *linkRegistryImpl) DebugForgetLink(linkId string) bool {
	self.linkMapLocks.Lock()
	defer self.linkMapLocks.Unlock()
	if link := self.linkByIdMap[linkId]; link != nil {
		delete(self.linkByIdMap, linkId)
		delete(self.linkMap, link.Key())
		return true
	}
	return false
}

func (self *linkRegistryImpl) LinkAccepted(link xlink.Xlink) (xlink.Xlink, bool) {
	self.Lock()
	defer self.Unlock()
	return self.applyLink(link)
}

func (self *linkRegistryImpl) DialSucceeded(link xlink.Xlink) (xlink.Xlink, bool) {
	self.Lock()
	defer self.Unlock()
	return self.applyLink(link)
}

func (self *linkRegistryImpl) applyLink(link xlink.Xlink) (xlink.Xlink, bool) {
	log := logrus.WithField("dest", link.DestinationId()).
		WithField("linkProtocol", link.LinkProtocol()).
		WithField("newLinkId", link.Id()).
		WithField("newLinkIteration", link.Iteration()).
		WithField("dialed", link.IsDialed())

	if link.IsClosed() {
		log.Info("link being registered, but is already closed, skipping registration")
		return nil, false
	}

	if existing, _ := self.GetLink(link.Key()); existing != nil {
		log = log.WithField("currentLinkId", existing.Id()).
			WithField("currentLinkIteration", existing.Iteration())

		if existing == link {
			log.Warn("link was re-applied, should not happen, not making any changes")
			debugz.DumpLocalStack()
			return nil, true
		}

		// if the id is the same we want to throw away the older one, since the new one is a replacement
		// If 5 duplicates have already come in, then clearly the other side is not happy with the
		// existing link, so try the new one instead
		if existing.Id() < link.Id() && existing.DuplicatesRejected() <= 5 {
			log.Info("duplicate link detected. closing other link (current link id is < than new link id)")

			// give the other side a chance to close the link first and report it as a duplicate
			time.AfterFunc(30*time.Second, func() {
				if err := link.Close(); err != nil {
					log.WithError(err).Error("error closing duplicate link")
				}
			})
			return existing, false
		}

		// make sure we don't block the registry loop
		go func() {
			log.Info("duplicate link detected. closing current link (current link id is >= than new link id)")

			legacyCtl := self.useLegacyLinkMgmtForOldCtrl()

			self.ctrls.ForEach(func(ctrlId string, ch channel.Channel) {
				// report link fault, then close link after allowing some time for circuits to be re-routed
				fault := &ctrl_pb.Fault{
					Id:        existing.Id(),
					Subject:   ctrl_pb.FaultSubject_LinkDuplicate,
					Iteration: existing.Iteration(),
				}

				if legacyCtl {
					fault.Subject = ctrl_pb.FaultSubject_LinkFault
				}

				if err := protobufs.MarshalTyped(fault).WithTimeout(time.Second).SendAndWaitForWire(ch); err != nil {
					log.WithField("ctrlId", ctrlId).
						WithError(err).
						Error("failed to send router fault when duplicate link detected")
				}
			})

			time.AfterFunc(time.Minute, func() {
				_ = existing.Close()
			})
		}()
	}

	self.linkMapLocks.Lock()
	self.linkMap[link.Key()] = link
	self.linkByIdMap[link.Id()] = link
	self.linkMapLocks.Unlock()

	self.updateLinkStateEstablished(link)

	log.Info("link registered")

	return nil, true
}

func (self *linkRegistryImpl) LinkClosed(link xlink.Xlink) {
	markLinkStateClosed := false
	self.linkMapLocks.Lock()
	if val := self.linkMap[link.Key()]; val == link {
		delete(self.linkMap, link.Key())
		markLinkStateClosed = true // only update link state to closed if this was the current link
	}

	if val := self.linkByIdMap[link.Id()]; val == link {
		delete(self.linkByIdMap, link.Id())
	}
	self.linkMapLocks.Unlock()

	if markLinkStateClosed {
		self.updateLinkStateClosed(link)
	} else {
		self.addLinkFaultForReplacedLink(link)
	}
}

func (self *linkRegistryImpl) Shutdown() {
	log := pfxlog.Logger()
	linkCount := 0
	for link := range self.Iter() {
		log.WithField("linkId", link.Id()).Info("closing link")
		_ = link.Close()
		linkCount++
	}
	log.WithField("linkCount", linkCount).Info("shutdown links in link registry")
}

func (self *linkRegistryImpl) SendRouterLinkMessage(link xlink.Xlink, channels ...channel.Channel) {
	linkMsg := &ctrl_pb.RouterLinks{
		Links: []*ctrl_pb.RouterLinks_RouterLink{
			{
				Id:           link.Id(),
				DestRouterId: link.DestinationId(),
				LinkProtocol: link.LinkProtocol(),
				DialAddress:  link.DialAddress(),
				Iteration:    link.Iteration(),
			},
		},
	}

	log := pfxlog.Logger().
		WithField("linkId", link.Id()).
		WithField("dest", link.DestinationId()).
		WithField("linkProtocol", link.LinkProtocol())

	if len(channels) == 0 {
		log.Info("no controllers available to notify of link")
	}

	for _, ch := range channels {
		if !capabilities.IsCapable(ch, capabilities.ControllerSingleRouterLinkSource) || link.IsDialed() {
			if err := protobufs.MarshalTyped(linkMsg).Send(ch); err != nil {
				log.WithError(err).Error("error sending router link message")
			}
			log.WithField("ctrlId", ch.Id()).Info("notified controller of new link")

		}
	}
}

/* XCtrl implementation so we get reconnect notifications */

func (self *linkRegistryImpl) LoadConfig(map[interface{}]interface{}) error {
	return nil
}

func (self *linkRegistryImpl) BindChannel(channel.Binding) error {
	return nil
}

func (self *linkRegistryImpl) Enabled() bool {
	return true
}

func (self *linkRegistryImpl) Run(env.RouterEnv) error {
	return nil
}

func (self *linkRegistryImpl) Iter() <-chan xlink.Xlink {
	self.linkMapLocks.RLock()
	result := make(chan xlink.Xlink, len(self.linkMap))
	self.linkMapLocks.RUnlock()

	go func() {
		self.linkMapLocks.RLock()
		defer self.linkMapLocks.RUnlock()

		for _, link := range self.linkMap {
			select {
			case result <- link:
			default:
			}
		}
		close(result)
	}()

	return result
}

func (self *linkRegistryImpl) NotifyOfReconnect(ch channel.Channel) {
	self.Lock()
	defer self.Unlock()

	pfxlog.Logger().WithField("ctrlId", ch.Id()).Info("resending link states after reconnect")
	alwaysSend := !capabilities.IsCapable(ch, capabilities.ControllerSingleRouterLinkSource)

	routerLinks := &ctrl_pb.RouterLinks{}
	for link := range self.Iter() {
		if alwaysSend || link.IsDialed() {
			routerLinks.Links = append(routerLinks.Links, &ctrl_pb.RouterLinks_RouterLink{
				Id:           link.Id(),
				DestRouterId: link.DestinationId(),
				LinkProtocol: link.LinkProtocol(),
				DialAddress:  link.DialAddress(),
				Iteration:    link.Iteration(),
			})
		}
	}

	if err := protobufs.MarshalTyped(routerLinks).Send(ch); err != nil {
		logrus.WithError(err).Error("failed to send router links on reconnect")
	}
}

func (self *linkRegistryImpl) GetTraceDecoders() []channel.TraceMessageDecoder {
	return nil
}

func (self *linkRegistryImpl) UpdateLinkDest(id string, version string, healthy bool, listeners []*ctrl_pb.Listener) {
	updateEvent := &linkDestUpdate{
		id:        id,
		version:   version,
		healthy:   healthy,
		listeners: listeners,
	}

	self.queueEvent(updateEvent)
}

func (self *linkRegistryImpl) RemoveLinkDest(id string) {
	self.queueEvent(&removeLinkDest{
		id: id,
	})
}

func (self *linkRegistryImpl) DialRequested(ctrlCh channel.Channel, dial *ctrl_pb.Dial) {
	self.queueEvent(&dialRequest{
		ctrlCh: ctrlCh,
		dial:   dial,
	})
}

func (self *linkRegistryImpl) markNewLinksNotified(links []stateAndLink) {
	self.queueEvent(&markNewLinksNotified{
		links: links,
	})
}

func (self *linkRegistryImpl) markFaultedLinksNotified(successfullySent []stateAndFaults) {
	self.queueEvent(&markFaultedLinksNotified{
		successfullySent: successfullySent,
	})
}

func (self *linkRegistryImpl) dialFailed(state *linkState) {
	self.queueEvent(&updateLinkStatusToDialFailed{
		linkState: state,
	})
}

func (self *linkRegistryImpl) queueEvent(evt event) {
	select {
	case <-self.env.GetCloseNotify():
	case self.events <- evt:
	}
}

func (self *linkRegistryImpl) run() {
	fullScanTicker := time.NewTicker(time.Minute)
	defer fullScanTicker.Stop()

	queueCheckTicker := time.NewTicker(5 * time.Second)
	defer queueCheckTicker.Stop()

	for {
		select {
		case evt := <-self.events:
			evt.Handle(self)
		case <-self.triggerNotifyC:
			self.notifyControllersOfLinks()
		case <-queueCheckTicker.C:
			self.evaluateLinkStateQueue()
			self.notifyControllersOfLinks()
		case <-fullScanTicker.C:
			self.evaluateDestinations()
		case <-self.env.GetCloseNotify():
			return
		}
	}
}

func (self *linkRegistryImpl) triggerNotify() {
	select {
	case self.triggerNotifyC <- struct{}{}:
	default:
	}
}

func (self *linkRegistryImpl) evaluateLinkStateQueue() {
	now := time.Now()
	for len(*self.linkStateQueue) > 0 {
		next := (*self.linkStateQueue)[0]
		if now.Before(next.nextDial) {
			return
		}
		heap.Pop(self.linkStateQueue)
		self.evaluateLinkState(next)
	}
}

func (self *linkRegistryImpl) evaluateDestinations() {
	for destId, dest := range self.destinations {
		hasEstablishedLinks := false
		for _, state := range dest.linkMap {
			// verify that links marked as established have an open link. There's a small chance that a link established
			// and link closed could be processed out of order if the event queue is full. This way, it will eventually
			// get fixed.
			if state.status == StatusEstablished {
				link, _ := self.GetLink(state.linkKey)
				if link == nil || link.IsClosed() {
					// If the link is not valid, allow it to be re-dialed
					state.retryDelay = time.Duration(0)
					state.nextDial = time.Now()
					state.updateStatus(StatusLinkFailed)
				} else {
					hasEstablishedLinks = true
				}
			}

			self.evaluateLinkState(state)
		}

		// we are notified of deleted routers. In case we're unreachable while a router is deleted,
		// we will also stop trying to contact unhealthy routers after a period. If a destination
		// has nothing to dial, it should also be removed
		if len(dest.linkMap) == 0 || (!dest.healthy && !hasEstablishedLinks && time.Since(dest.unhealthyAt) > 48*time.Hour) {
			delete(self.destinations, destId)
		}
	}
}

func (self *linkRegistryImpl) evaluateLinkState(state *linkState) {
	log := pfxlog.Logger().WithField("key", state.linkKey)

	couldDial := state.status != StatusEstablished && state.status != StatusDialing && state.nextDial.Before(time.Now())

	if couldDial && state.dialActive.CompareAndSwap(false, true) {
		state.updateStatus(StatusDialing)
		iteration := state.dialAttempts.Add(1)
		log = log.WithField("linkId", state.linkId).WithField("iteration", iteration)
		log.Info("queuing link to dial")

		err := self.env.GetLinkDialerPool().QueueOrError(func() {
			defer state.dialActive.Store(false)

			link, _ := self.GetLink(state.linkKey)
			if link != nil {
				log.Info("link already present, attempting to mark established")
				self.updateLinkStateEstablished(link)
				return
			}

			log.Info("dialing link")
			link, err := state.dialer.Dial(state)
			if err != nil {
				log.WithError(err).Error("error dialing link")
				self.dialFailed(state)
				return
			}

			existing, success := self.DialSucceeded(link)
			if !success && existing == nil {
				self.dialFailed(state)
			}
		})
		if err != nil {
			state.dialActive.Store(false)
			log.WithError(err).Error("unable to queue link dial, see pool error")
			state.updateStatus(StatusQueueFailed)
			state.dialFailed(self)
		}
	}
}

func (self *linkRegistryImpl) updateLinkStateEstablished(link xlink.Xlink) {
	self.queueEvent(&updateLinkStatusForLink{
		link:   link,
		status: StatusEstablished,
	})
}

func (self *linkRegistryImpl) updateLinkStateClosed(link xlink.Xlink) {
	self.queueEvent(&updateLinkStatusForLink{
		link:   link,
		status: StatusLinkFailed,
	})
}

func (self *linkRegistryImpl) addLinkFaultForReplacedLink(link xlink.Xlink) {
	self.queueEvent(&addLinkFaultForReplacedLink{
		link: link,
	})
}

func (self *linkRegistryImpl) Inspect(timeout time.Duration) *inspect.LinksInspectResult {
	evt := &inspectLinkStatesEvent{
		result: atomic.Pointer[[]*inspect.LinkDest]{},
		done:   make(chan struct{}),
	}
	self.queueEvent(evt)

	result := &inspect.LinksInspectResult{}
	for link := range self.Iter() {
		result.Links = append(result.Links, link.InspectLink())
	}

	var err error
	result.Destinations, err = evt.GetResults(timeout)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	return result
}

func (self *linkRegistryImpl) useLegacyLinkMgmtForOldCtrl() bool {
	legacyCtrl := false

	for _, ctrl := range self.ctrls.GetAll() {
		if ok, _ := ctrl.GetVersion().HasMinimumVersion("0.30.0"); !ok {
			legacyCtrl = true
		}
	}
	return legacyCtrl
}

func (self *linkRegistryImpl) GetLinkKey(dialerBinding, protocol, dest, listenerBinding string) string {
	legacyCtrl := self.useLegacyLinkMgmtForOldCtrl()
	if dialerBinding == "" || legacyCtrl {
		dialerBinding = "default"
	}

	if listenerBinding == "" || legacyCtrl {
		listenerBinding = "default"
	}

	return fmt.Sprintf("%s->%s:%s->%s", dialerBinding, protocol, dest, listenerBinding)
}

func (self *linkRegistryImpl) notifyControllersOfLinks() {
	if self.notifyInProgress.Load() {
		pfxlog.Logger().WithField("op", "link-notify").Info("new link notification already in progress, exiting")
		return
	}

	var links []stateAndLink
	var faults []stateAndFaults

	for _, dest := range self.destinations {
		for _, state := range dest.linkMap {
			if !state.ctrlsNotified {
				if state.status == StatusEstablished {
					link, _ := self.GetLink(state.linkKey)
					if link == nil {
						pfxlog.Logger().
							WithField("op", "link-notify").
							WithField("linkId", state.linkId).
							Info("link not found for link key on established link, marking failed")
						state.updateStatus(StatusDialFailed)
						state.dialFailed(self)
					} else if link.IsDialed() {
						links = append(links, stateAndLink{
							state: state,
							link:  link,
						})
					}

					// if we have an established link, don't send faults for it
					if link != nil {
						state.clearFaultsForLinkId(link.Id())
					}
				}
			}

			if len(state.linkFaults) > 0 {
				var linkFaults []linkFault
				linkFaults = append(linkFaults, state.linkFaults...)
				faults = append(faults, stateAndFaults{
					state:  state,
					faults: linkFaults,
				})
			}
		}
	}

	if len(links) == 0 && len(faults) == 0 {
		return
	}

	pfxlog.Logger().WithField("op", "link-notify").Info("attempting to queue link notifications")
	self.notifyInProgress.Store(true)
	err := self.env.GetRateLimiterPool().QueueOrError(func() {
		pfxlog.Logger().WithField("op", "link-notify").Info("link notifications starting")

		defer func() {
			self.notifyInProgress.Store(false)
			pfxlog.Logger().WithField("op", "link-notify").Info("link notifications exiting")
		}()

		if len(links) > 0 {
			self.sendNewLinks(links)
		}

		if len(faults) > 0 {
			self.sendLinkFaults(faults)
		}
	})

	if err != nil {
		pfxlog.Logger().WithField("op", "link-notify").Info("unable to queue link notifications")
		self.notifyInProgress.Store(false)
	}
}

func (self *linkRegistryImpl) sendNewLinks(links []stateAndLink) {
	routerLinks := &ctrl_pb.RouterLinks{}
	for _, pair := range links {
		link := pair.link
		routerLinks.Links = append(routerLinks.Links, &ctrl_pb.RouterLinks_RouterLink{
			Id:           link.Id(),
			DestRouterId: link.DestinationId(),
			LinkProtocol: link.LinkProtocol(),
			DialAddress:  link.DialAddress(),
			Iteration:    link.Iteration(),
		})
	}

	allSent := true
	for ctrlId, ctrl := range self.ctrls.GetAll() {
		connectedChecker := ctrl.Channel().Underlay().(interface{ IsConnected() bool })
		log := pfxlog.Logger().WithField("ctrlId", ctrlId).WithField("op", "link-notify")
		if connectedChecker.IsConnected() {
			msgEnv := protobufs.MarshalTyped(routerLinks).WithTimeout(10 * time.Second)
			if err := msgEnv.SendAndWaitForWire(ctrl.Channel()); err != nil {
				log.WithError(err).Error("timeout sending new router links")
				allSent = false

				for _, pair := range links {
					log.WithError(err).
						WithField("linkId", pair.link.Id()).
						WithField("iteration", pair.link.Iteration()).
						Info("failed to notify controller of new link")
				}
			} else {
				for _, pair := range links {
					log.WithField("linkId", pair.link.Id()).
						WithField("iteration", pair.link.Iteration()).
						Info("notified controller of new link")
				}
			}
		}
	}

	if allSent {
		self.markNewLinksNotified(links)
	}
}

func (self *linkRegistryImpl) sendLinkFaults(list []stateAndFaults) {
	var successfullySent []stateAndFaults
	for _, item := range list {
		var sent []linkFault

		for _, fault := range item.faults {
			allSent := true
			for ctrlId, ctrl := range self.ctrls.GetAll() {
				faultMsg := &ctrl_pb.Fault{
					Subject:   ctrl_pb.FaultSubject_LinkFault,
					Id:        fault.linkId,
					Iteration: fault.iteration,
				}

				log := pfxlog.Logger().WithField("ctrlId", ctrlId).
					WithField("op", "link-notify").
					WithField("linkId", fault.linkId).
					WithField("iteration", fault.iteration)

				connectedChecker := ctrl.Channel().Underlay().(interface{ IsConnected() bool })

				if connectedChecker.IsConnected() {
					msgEnv := protobufs.MarshalTyped(faultMsg).WithTimeout(10 * time.Second)
					if err := msgEnv.SendAndWaitForWire(ctrl.Channel()); err != nil {
						log.WithError(err).Error("timeout sending link fault")
						allSent = false
						log.Info("failed to notify controller of link fault")
					} else {
						log.Info("notified controller of link fault")
					}
				} else if ctrl.TimeSinceLastContact() < 2*time.Minute {
					// if this is a brief outage, need to keep trying, otherwise there are potential race conditions
					allSent = false
				}
			}
			if allSent {
				sent = append(sent, fault)
			}
		}
		if len(sent) > 0 {
			successfullySent = append(successfullySent, stateAndFaults{
				state:  item.state,
				faults: sent,
			})
		}
	}

	if len(successfullySent) > 0 {
		self.markFaultedLinksNotified(successfullySent)
	}
}

type stateAndLink struct {
	state *linkState
	link  xlink.Xlink
}

type stateAndFaults struct {
	state  *linkState
	faults []linkFault
}
