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
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/inspect"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/env"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/sirupsen/logrus"
	"sync"
	"sync/atomic"
	"time"
)

type Env interface {
	GetNetworkControllers() env.NetworkControllers
	GetXlinkDialers() []xlink.Dialer
	GetCloseNotify() <-chan struct{}
	GetLinkDialerPool() goroutines.Pool
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
	}

	go result.run()

	return result
}

type linkRegistryImpl struct {
	linkMap     map[string]xlink.Xlink
	linkByIdMap map[string]xlink.Xlink
	sync.Mutex
	ctrls env.NetworkControllers

	env            Env
	destinations   map[string]*linkDest
	linkStateQueue *linkStateHeap
	events         chan event
}

func (self *linkRegistryImpl) GetLink(linkKey string) (xlink.Xlink, bool) {
	self.Lock()
	defer self.Unlock()

	val, found := self.linkMap[linkKey]
	return val, found
}

func (self *linkRegistryImpl) GetLinkById(linkId string) (xlink.Xlink, bool) {
	self.Lock()
	defer self.Unlock()

	link, found := self.linkByIdMap[linkId]
	return link, found
}

func (self *linkRegistryImpl) DebugForgetLink(linkId string) bool {
	self.Lock()
	defer self.Unlock()
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
		WithField("newLinkId", link.Id())

	if link.IsClosed() {
		log.Info("link being registered, but is already closed, skipping registration")
		return nil, false
	}
	if existing := self.linkMap[link.Key()]; existing != nil {
		log = log.WithField("currentLinkId", existing.Id())
		if existing.Id() < link.Id() {
			// give the other side a chance to close the link first and report it as a duplicate
			time.AfterFunc(30*time.Second, func() {
				if err := link.Close(); err != nil {
					log.WithError(err).Error("error closing duplicate link")
				}
			})
			return existing, false
		}
		log.Info("duplicate link detected. closing current link (current link id is > than new link id)")

		legacyCtl := self.useLegacyLinkMgmtForOldCtrl()

		self.ctrls.ForEach(func(ctrlId string, ch channel.Channel) {
			// report link fault, then close link after allowing some time for circuits to be re-routed
			fault := &ctrl_pb.Fault{
				Id:      existing.Id(),
				Subject: ctrl_pb.FaultSubject_LinkDuplicate,
			}

			if legacyCtl {
				fault.Subject = ctrl_pb.FaultSubject_LinkFault
			}

			if err := protobufs.MarshalTyped(fault).Send(ch); err != nil {
				log.WithField("ctrlId", ctrlId).
					WithError(err).
					Error("failed to send router fault when duplicate link detected")
			}
		})

		time.AfterFunc(5*time.Minute, func() {
			_ = existing.Close()
		})
	}
	self.linkMap[link.Key()] = link
	self.linkByIdMap[link.Id()] = link
	self.updateLinkStateEstablished(link)
	self.SendRouterLinkMessage(link, self.ctrls.AllResponsiveCtrlChannels()...)
	return nil, true
}

func (self *linkRegistryImpl) LinkClosed(link xlink.Xlink) {
	self.Lock()
	defer self.Unlock()
	if val := self.linkMap[link.Key()]; val == link {
		delete(self.linkMap, link.Key())
	}
	delete(self.linkByIdMap, link.Id())
	self.updateLinkStateClosed(link)
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
			},
		},
	}

	log := pfxlog.Logger().
		WithField("linkId", link.Id()).
		WithField("dest", link.DestinationId()).
		WithField("linkProtocol", link.LinkProtocol())

	for _, ch := range channels {
		if err := protobufs.MarshalTyped(linkMsg).Send(ch); err != nil {
			log.WithError(err).Error("error sending router link message")
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
	result := make(chan xlink.Xlink, len(self.linkMap))
	go func() {
		self.Lock()
		defer self.Unlock()

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
	routerLinks := &ctrl_pb.RouterLinks{}
	for link := range self.Iter() {
		routerLinks.Links = append(routerLinks.Links, &ctrl_pb.RouterLinks_RouterLink{
			Id:           link.Id(),
			DestRouterId: link.DestinationId(),
			LinkProtocol: link.LinkProtocol(),
			DialAddress:  link.DialAddress(),
		})
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
		case <-queueCheckTicker.C:
			self.evaluateLinkStateQueue()
		case <-fullScanTicker.C:
			self.evaluateDestinations()
		case <-self.env.GetCloseNotify():
			return
		}
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
	for _, dest := range self.destinations {
		// TODO: When do we drop destinations? Should we ask the controller after the router has been
		//       unhealthy for a while and it doesn't have any established links? Do this on exponential backoff?
		//      Should the controller send router removed messages?
		for _, state := range dest.linkMap {
			self.evaluateLinkState(state)
		}
	}
}

func (self *linkRegistryImpl) evaluateLinkState(state *linkState) {
	log := pfxlog.Logger().WithField("key", state.linkKey)

	couldDial := state.status != StatusEstablished && state.status != StatusDialing && state.nextDial.Before(time.Now())

	if couldDial {
		state.status = StatusDialing
		state.dialAttempts++

		err := self.env.GetLinkDialerPool().QueueOrError(func() {
			link, _ := self.GetLink(state.linkKey)
			if link != nil {
				log.Warn("link already present, but link status still pending")
				return
			}

			link, err := state.dialer.Dial(state)
			if err != nil {
				log.WithError(err).Error("error dialing link")
				self.queueEvent(&updateLinkState{
					linkState: state,
					status:    StatusDialFailed,
				})
				return
			}
			self.DialSucceeded(link)
		})
		if err != nil {
			log.WithError(err).Error("unable to queue link dial, see pool error")
			self.queueEvent(&updateLinkState{
				linkState: state,
				status:    StatusQueueFailed,
			})
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
