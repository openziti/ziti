package router

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/env"
	"github.com/openziti/fabric/router/xlink"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

func NewLinkRegistry(ctrls env.NetworkControllers) *linkRegistryImpl {
	return &linkRegistryImpl{
		linkMap:     map[string]xlink.Xlink{},
		linkByIdMap: map[string]xlink.Xlink{},
		dialLocks:   map[string]int64{},
		ctrls:       ctrls,
	}
}

type linkRegistryImpl struct {
	linkMap     map[string]xlink.Xlink
	linkByIdMap map[string]xlink.Xlink
	dialLocks   map[string]int64
	sync.Mutex
	ctrls env.NetworkControllers
}

func (self *linkRegistryImpl) GetLink(routerId, linkProtocol string) (xlink.Xlink, bool) {
	self.Lock()
	defer self.Unlock()

	key := self.getLookupKey(routerId, linkProtocol)
	val, found := self.linkMap[key]
	if found {
		return val, true
	}
	return nil, false
}

func (self *linkRegistryImpl) GetLinkById(linkId string) (xlink.Xlink, bool) {
	self.Lock()
	defer self.Unlock()

	link, found := self.linkByIdMap[linkId]
	return link, found
}

func (self *linkRegistryImpl) GetDialLock(dial xlink.Dial) (xlink.Xlink, bool) {
	self.Lock()
	defer self.Unlock()
	key := self.getDialLookupKey(dial)

	val, found := self.linkMap[key]
	if found {
		self.sendRouterLinkMessage(self.notifySingle(dial.GetCtrlId()), val)
		return val, false
	}

	if val, found := self.dialLocks[key]; found {
		if val > time.Now().Add(-time.Minute).UnixMilli() {
			return nil, false
		}
	}

	if len(self.dialLocks) > 100 {
		self.purgeOldDialLocks()
	}

	self.dialLocks[key] = time.Now().UnixMilli()
	return nil, true
}

func (self *linkRegistryImpl) purgeOldDialLocks() {
	count := 0
	minuteAgo := time.Now().Add(-time.Minute).UnixMilli()
	for k, v := range self.dialLocks {
		if v < minuteAgo {
			delete(self.dialLocks, k)
			count++
		}
	}
	if count > 0 {
		logrus.WithField("locksDelete", count).Warn("found old link dial locks")
	}
}

func (self *linkRegistryImpl) getDialLookupKey(dial xlink.Dial) string {
	return self.getLookupKey(dial.GetRouterId(), dial.GetLinkProtocol())
}

func (self *linkRegistryImpl) getLinkLookupKey(link xlink.Xlink) string {
	return self.getLookupKey(link.DestinationId(), link.LinkProtocol())
}

func (self *linkRegistryImpl) getLookupKey(routerId, linkProtocol string) string {
	key := fmt.Sprintf("%v#%v", routerId, linkProtocol)
	return key
}

func (self *linkRegistryImpl) DialFailed(dial xlink.Dial) {
	self.Lock()
	defer self.Unlock()
	key := self.getDialLookupKey(dial)
	delete(self.dialLocks, key)
}

func (self *linkRegistryImpl) DebugForgetLink(linkId string) bool {
	self.Lock()
	defer self.Unlock()
	if link := self.linkByIdMap[linkId]; link != nil {
		key := self.getLinkLookupKey(link)
		delete(self.linkByIdMap, linkId)
		delete(self.linkMap, key)
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
	key := self.getLinkLookupKey(link)
	delete(self.dialLocks, key)
	return self.applyLink(link)
}

func (self *linkRegistryImpl) applyLink(link xlink.Xlink) (xlink.Xlink, bool) {
	log := logrus.WithField("dest", link.DestinationId()).
		WithField("linkProtocol", link.LinkProtocol()).
		WithField("newLinkId", link.Id())

	key := self.getLinkLookupKey(link)
	if link.IsClosed() {
		log.Info("link being registered, but is already closed, skipping registration")
		return nil, false
	}
	if existing := self.linkMap[key]; existing != nil {
		log = log.WithField("currentLinkId", existing.Id())
		if existing.Id() < link.Id() {
			log.Info("duplicate link detected. closing new link (new link id is > than current link id)")
			self.sendRouterLinkMessage(self.ctrls.ForEach, existing)
			if err := link.Close(); err != nil {
				log.WithError(err).Error("error closing duplicate link")
			}
			return existing, false
		}
		log.Info("duplicate link detected. closing current link (current link id is > than new link id)")

		self.ctrls.ForEach(func(ctrlId string, ch channel.Channel) {
			// report link fault, then close link after allowing some time for circuits to be re-routed
			fault := &ctrl_pb.Fault{
				Id:      existing.Id(),
				Subject: ctrl_pb.FaultSubject_LinkFault,
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
	self.linkMap[key] = link
	self.linkByIdMap[link.Id()] = link
	return nil, true
}

func (self *linkRegistryImpl) LinkClosed(link xlink.Xlink) {
	self.Lock()
	defer self.Unlock()
	key := self.getLinkLookupKey(link)
	if val := self.linkMap[key]; val == link {
		delete(self.linkMap, key)
	}
	delete(self.linkByIdMap, link.Id())
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

func (self *linkRegistryImpl) sendRouterLinkMessage(notifier notifierF, link xlink.Xlink) {
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

	notifier(func(ctrlId string, ch channel.Channel) {
		if err := protobufs.MarshalTyped(linkMsg).Send(ch); err != nil {
			log.WithError(err).Error("error sending router link message")
		}
	})
}

type notifierF func(func(ctrlId string, ch channel.Channel))

func (self *linkRegistryImpl) notifySingle(ctrlId string) notifierF {
	return func(f func(ctrlId string, ch channel.Channel)) {
		if ch := self.ctrls.GetCtrlChannel(ctrlId); ch != nil {
			f(ctrlId, ch)
		} else {
			pfxlog.Logger().WithField("ctrlId", ctrlId).Error("control channel for controller not available")
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

func (self *linkRegistryImpl) Run(env env.RouterEnv) error {
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
