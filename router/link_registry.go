package router

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/storage/boltz"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

func NewLinkRegistry() xlink.Registry {
	return &linkRegistryImpl{
		linkMap:     map[string]xlink.Xlink{},
		linkByIdMap: map[string]xlink.Xlink{},
		dialLocks:   map[string]int64{},
	}
}

type linkRegistryImpl struct {
	linkMap     map[string]xlink.Xlink
	linkByIdMap map[string]xlink.Xlink
	dialLocks   map[string]int64
	sync.Mutex
	ctrlCh channel.Channel
}

func (self *linkRegistryImpl) ControlChannel() channel.Channel {
	// we may get link requests before the control channel is fully
	// established. wait until it's set before we return. Will only
	// happen right at startup
	for self.ctrlCh == nil {
		time.Sleep(30 * time.Millisecond)
	}
	return self.ctrlCh
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
		self.sendRouterLinkMessage(val)
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
		WithField("newLinkId", link.Id().Token)

	key := self.getLinkLookupKey(link)
	if link.IsClosed() {
		log.Info("link being registered, but is already closed, skipping registration")
		return nil, false
	}
	if existing := self.linkMap[key]; existing != nil {
		log = log.WithField("currentLinkId", existing.Id().Token)
		if existing.Id().Token < link.Id().Token {
			log.Info("duplicate link detected. closing new link (new link id is > than current link id)")
			self.sendRouterLinkMessage(existing)
			if err := link.Close(); err != nil {
				log.WithError(err).Error("error closing duplicate link")
			}
			return existing, false
		}
		log.Info("duplicate link detected. closing current link (current link id is > than new link id)")

		// report link fault, then close link after allowing some time for circuits to be re-routed
		fault := &ctrl_pb.Fault{
			Id:      existing.Id().Token,
			Subject: ctrl_pb.FaultSubject_LinkFault,
		}

		if err := protobufs.MarshalTyped(fault).Send(self.ControlChannel()); err != nil {
			logrus.WithError(err).Error("failed to send router fault when duplicate link detected")
		}

		time.AfterFunc(5*time.Minute, func() {
			_ = existing.Close()
		})
	}
	self.linkMap[key] = link
	self.linkByIdMap[link.Id().Token] = link
	return nil, true
}

func (self *linkRegistryImpl) LinkClosed(link xlink.Xlink) {
	self.Lock()
	defer self.Unlock()
	key := self.getLinkLookupKey(link)
	if val := self.linkMap[key]; val == link {
		delete(self.linkMap, key)
	}
	delete(self.linkByIdMap, link.Id().Token)
}

func (self *linkRegistryImpl) Shutdown() {
	log := pfxlog.Logger()
	linkCount := 0
	for link := range self.Iter() {
		log.WithField("linkId", link.Id().Token).Info("closing link")
		_ = link.Close()
		linkCount++
	}
	log.WithField("linkCount", linkCount).Info("shutdown links in link registry")
}

func (self *linkRegistryImpl) sendRouterLinkMessage(link xlink.Xlink) {
	linkMsg := &ctrl_pb.RouterLinks{
		Links: []*ctrl_pb.RouterLinks_RouterLink{
			{
				Id:           link.Id().Token,
				DestRouterId: link.DestinationId(),
				LinkProtocol: link.LinkProtocol(),
			},
		},
	}
	if err := protobufs.MarshalTyped(linkMsg).Send(self.ControlChannel()); err != nil {
		pfxlog.Logger().WithField("linkId", link.Id().Token).
			WithField("dest", link.DestinationId()).
			WithField("linkProtocol", link.LinkProtocol()).
			WithError(err).Error("error sending router link message")
	}
}

/* XCtrl implementation so we get reconnect notifications */

func (self *linkRegistryImpl) LoadConfig(map[interface{}]interface{}) error {
	return nil
}

func (self *linkRegistryImpl) BindChannel(binding channel.Binding) error {
	self.ctrlCh = binding.GetChannel()
	return nil
}

func (self *linkRegistryImpl) Enabled() bool {
	return true
}

func (self *linkRegistryImpl) Run(channel.Channel, boltz.Db, chan struct{}) error {
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

func (self *linkRegistryImpl) NotifyOfReconnect() {
	routerLinks := &ctrl_pb.RouterLinks{}
	for link := range self.Iter() {
		routerLinks.Links = append(routerLinks.Links, &ctrl_pb.RouterLinks_RouterLink{
			Id:           link.Id().Token,
			DestRouterId: link.DestinationId(),
			LinkProtocol: link.LinkProtocol(),
		})
	}

	if err := protobufs.MarshalTyped(routerLinks).Send(self.ControlChannel()); err != nil {
		logrus.WithError(err).Error("failed to send router links on reconnect")
	}
}

func (self *linkRegistryImpl) GetTraceDecoders() []channel.TraceMessageDecoder {
	return nil
}
