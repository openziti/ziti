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

package xlink_transport

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/transport/v2"
	fabricMetrics "github.com/openziti/ziti/v2/common/metrics"
	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type listener struct {
	id                 *identity.TokenId
	config             *listenerConfig
	listener           io.Closer
	accepter           xlink.Acceptor
	bindHandlerFactory BindHandlerFactory
	tcfg               transport.Configuration
	pendingLinks       map[string]*pendingLink
	lock               sync.Mutex
	env                LinkEnv
	xlinkRegistery     xlink.Registry

	stopC  chan struct{}
	closed atomic.Bool
}

func (self *listener) Listen() error {
	config := channel.ListenerConfig{
		ConnectOptions:     self.config.options.ConnectOptions,
		TransportConfig:    self.tcfg,
		PoolConfigurator:   fabricMetrics.GoroutinesPoolMetricsConfigF(self.env.GetMetricsRegistry(), "pool.listener.link"),
		ConnectionHandlers: []channel.ConnectionHandler{&ConnectionHandler{self.id}},
		MessageStrategy:    channel.DatagramMessageStrategy(xgress.UnmarshallPacketPayload),
	}

	acceptor := channel.NewMultiListener(self.handleGroupedUnderlay, self.handleUngroupedNewUnderlay)

	var err error
	if self.listener, err = channel.NewClassicListenerF(self.id, self.config.bind, config, acceptor.AcceptUnderlay); err != nil {
		return fmt.Errorf("error listening (%w)", err)
	}

	go self.cleanupExpiredPartialLinks()
	return nil
}

func (self *listener) GetAdvertisement() string {
	return self.config.advertise.String()
}

func (self *listener) GetLinkProtocol() string {
	return self.config.linkProtocol
}

func (self *listener) GetLinkCostTags() []string {
	return self.config.linkCostTags
}

func (self *listener) GetGroups() []string {
	return self.config.groups
}

// Close shuts down the listener: stops the cleanup goroutine, closes the
// listening socket, and closes any partial (not-yet-fully-accepted) links.
// Accepted Xlinks are independent channels at this point and are NOT closed
// — they continue to operate until naturally torn down or until an operator
// removes them via `ziti fabric delete link`. Safe to call multiple times.
func (self *listener) Close() error {
	if self.closed.CompareAndSwap(false, true) {
		close(self.stopC)
		var err error
		if self.listener != nil {
			err = self.listener.Close()
		}

		// Drop any partial (pre-accept) links the cleanup goroutine would
		// otherwise have eventually expired. They hold underlay resources
		// that should release immediately when the listener goes away.
		self.lock.Lock()
		defer self.lock.Unlock()
		for k, v := range self.pendingLinks {
			_ = v.link.Close()
			delete(self.pendingLinks, k)
		}

		return err
	}
	return nil
}

func (self *listener) GetLocalBinding() string {
	return self.config.bindInterface
}

func (self *listener) handleGroupedUnderlay(underlay channel.Underlay, closeCallback func()) (channel.MultiChannel, error) {
	linkChannel := NewListenerLinkChannel(underlay, self.env.GetLinkPayloadSenderQueueSize(), self.env.GetLinkAckSenderQueueSize())
	multiConfig := channel.MultiChannelConfig{
		LogicalName:     "link/" + resolveLinkId(underlay.Headers(), underlay.Id()),
		Options:         self.config.options,
		UnderlayHandler: linkChannel,
		BindHandler: channel.BindHandlerF(func(binding channel.Binding) error {
			binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
				closeCallback()
			}))
			return self.BindChannel(binding)
		}),
		Underlay: underlay,
	}
	mc, err := channel.NewMultiChannel(&multiConfig)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("failure accepting link channel %v with mult-underlay", underlay.Label())
		return nil, err
	}

	return mc, nil
}

func (self *listener) handleUngroupedNewUnderlay(underlay channel.Underlay) error {
	if _, err := channel.NewChannelWithUnderlay("link", underlay, self, self.config.options); err != nil {
		logrus.WithError(err).Error("error creating link channel")
		return err
	}
	return nil
}

func (self *listener) BindChannel(binding channel.Binding) error {
	headers := channel.Headers(binding.GetChannel().Underlay().Headers())
	linkId := resolveLinkId(headers, binding.GetChannel().Id())

	log := pfxlog.ChannelLogger("link", "linkListener").
		WithField("linkProtocol", self.GetLinkProtocol()).
		WithField("linkId", linkId)

	var chanType channelType

	routerId := ""
	routerVersion := ""
	dialerBinding := ""
	var iteration uint32

	if headers != nil {
		var ok bool
		if routerId, ok = headers.GetStringHeader(LinkHeaderRouterId); ok {
			log = log.WithField("routerId", routerId)
		}
		if val, ok := headers.GetByteHeader(LinkHeaderType); ok {
			chanType = channelType(val)
		}
		if routerVersion, ok = headers.GetStringHeader(LinkHeaderRouterVersion); ok {
			log = log.WithField("routerVersion", routerVersion)
		}
		if dialerBinding, ok = headers.GetStringHeader(LinkHeaderBinding); ok {
			log = log.WithField("dialerBinding", dialerBinding)
		}
		if val, ok := headers.GetUint32Header(LinkHeaderIteration); ok {
			iteration = val
			log = log.WithField("iteration", iteration)
		}
	}

	log.Info("binding link channel")

	linkMeta := &linkMetadata{
		linkId:        linkId,
		routerId:      routerId,
		routerVersion: routerVersion,
		dialerBinding: dialerBinding,
		iteration:     iteration,
	}

	if chanType != 0 {
		log = log.WithField("channelType", chanType)
		return self.bindSplitChannel(binding, chanType, linkMeta, log)
	}

	return self.bindNonSplitChannel(binding, linkMeta, log)
}

func (self *listener) bindSplitChannel(binding channel.Binding, chanType channelType, linkMeta *linkMetadata, log *logrus.Entry) error {
	headers := binding.GetChannel().Underlay().Headers()
	connId, ok := channel.Headers(headers).GetStringHeader(LinkHeaderConnId)
	if !ok {
		return errors.New("split conn received but missing connection id. closing")
	}

	log = log.WithField("connId", connId)
	log.Info("accepted part of split conn")

	complete, xli, err := self.getOrCreateSplitLink(connId, linkMeta, binding, chanType)
	if err != nil {
		log.WithError(err).Error("error binding link channel")
		return err
	}

	latencyPing := chanType == PayloadChannel
	if err = self.bindHandlerFactory.NewBindHandler(xli, latencyPing, true).BindChannel(binding); err != nil {
		self.cleanupDeadPartialLink(connId)
		if closeErr := xli.Close(); closeErr != nil {
			log.WithError(closeErr).Error("error closing partial split link")
		}
		return err
	}

	if complete && xli.payloadCh != nil && xli.ackCh != nil {
		if err = self.accepter.Accept(xli); err != nil {
			log.WithError(err).Error("error accepting incoming Xlink")

			if err := xli.Close(); err != nil {
				log.WithError(err).Debugf("error closing link")
			}
			return err
		}
		log.Info("accepted link")

		if existingLink, applied := self.xlinkRegistery.LinkAccepted(xli); applied {
			log.Info("link registered")
		} else if existingLink != nil {
			log.WithField("existingLinkId", existingLink.Id()).Info("existing link found, new link closed")
		}
	}

	return nil
}

func (self *listener) cleanupDeadPartialLink(id string) {
	self.lock.Lock()
	defer self.lock.Unlock()

	delete(self.pendingLinks, id)
}

func (self *listener) getOrCreateSplitLink(connId string, linkMeta *linkMetadata, binding channel.Binding, chanType channelType) (bool, *splitImpl, error) {
	self.lock.Lock()
	defer self.lock.Unlock()

	complete := false
	var link *splitImpl

	if pending, found := self.pendingLinks[connId]; found {
		delete(self.pendingLinks, connId)
		link = pending.link
		complete = true
	} else {
		pending = &pendingLink{
			link: &splitImpl{
				id:            linkMeta.linkId,
				key:           self.xlinkRegistery.GetLinkKey(linkMeta.dialerBinding, self.GetLinkProtocol(), linkMeta.routerId, self.config.bindInterface),
				routerId:      linkMeta.routerId,
				routerVersion: linkMeta.routerVersion,
				linkProtocol:  self.GetLinkProtocol(),
				dialAddress:   self.GetAdvertisement(),
				iteration:     linkMeta.iteration,
				dialed:        false,
			},
			eventTime: time.Now(),
		}
		self.pendingLinks[connId] = pending
		link = pending.link
	}

	if chanType == PayloadChannel {
		if err := link.syncInit(func() error {
			if link.payloadCh == nil {
				link.payloadCh = binding.GetChannel()
				return nil
			}
			return errors.Errorf("got two payload channels for link %v", linkMeta.linkId)
		}); err != nil {
			return false, nil, err
		}
	} else if chanType == AckChannel {
		if err := link.syncInit(func() error {
			if link.ackCh == nil {
				link.ackCh = binding.GetChannel()
				return nil
			}
			return errors.Errorf("got two ack channels for link %v", linkMeta.linkId)
		}); err != nil {
			return false, nil, err
		}
	} else {
		return false, nil, errors.Errorf("invalid channel type %v", chanType)
	}

	return complete, link, nil
}

func (self *listener) bindNonSplitChannel(binding channel.Binding, linkMeta *linkMetadata, log *logrus.Entry) error {
	xli := &impl{
		id:            linkMeta.linkId,
		key:           self.xlinkRegistery.GetLinkKey(linkMeta.dialerBinding, self.GetLinkProtocol(), linkMeta.routerId, self.config.bindInterface),
		routerId:      linkMeta.routerId,
		routerVersion: linkMeta.routerVersion,
		linkProtocol:  self.GetLinkProtocol(),
		dialAddress:   self.GetAdvertisement(),
		iteration:     linkMeta.iteration,
		dialed:        false,
	}

	if mc, ok := binding.GetChannel().(channel.MultiChannel); ok {
		if linkChan, ok := mc.GetUnderlayHandler().(LinkChannel); ok {
			xli.ch = linkChan
		}
	}

	if xli.ch == nil {
		xli.ch = NewSingleLinkChannel(binding.GetChannel())
	}

	bindHandler := self.bindHandlerFactory.NewBindHandler(xli, true, true)
	if err := bindHandler.BindChannel(binding); err != nil {
		return errors.Wrapf(err, "error binding channel for link [l/%v]", linkMeta.linkId)
	}

	log.Info("accepting link")

	if err := self.accepter.Accept(xli); err != nil {
		log.WithError(err).Error("error accepting incoming Xlink")
		if err := xli.Close(); err != nil {
			log.WithError(err).Debugf("error closing link")
		}
		return err
	}

	if existingLink, applied := self.xlinkRegistery.LinkAccepted(xli); applied {
		log.Info("link registered")
	} else if existingLink != nil {
		log.WithField("existingLinkId", existingLink.Id()).Info("existing link found, new link closed")
	}

	log.Info("accepted link")
	return nil
}

func (self *listener) cleanupExpiredPartialLinks() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			func() {
				self.lock.Lock()
				defer self.lock.Unlock()
				now := time.Now()
				for k, v := range self.pendingLinks {
					if now.Sub(v.eventTime) > (30 * time.Second) {
						_ = v.link.Close()
						delete(self.pendingLinks, k)
					}
				}
			}()
		case <-self.stopC:
			return
		case <-self.env.GetCloseNotify():
			return
		}
	}
}

type pendingLink struct {
	link      *splitImpl
	eventTime time.Time
}

type linkMetadata struct {
	linkId        string
	routerId      string
	routerVersion string
	dialerBinding string
	iteration     uint32
}

// resolveLinkId returns the link id for an incoming link connection. It prefers
// the LinkHeaderLinkId header, falling back to the given id (the channel identity
// token) when the header isn't present, as is the case for older routers. Dialing
// routers currently send the link id as both; reading the header here is what will
// allow the channel identity to become the dialing router's actual id in a future
// release.
func resolveLinkId(headers map[int32][]byte, fallbackId string) string {
	if linkId, ok := channel.Headers(headers).GetStringHeader(LinkHeaderLinkId); ok && linkId != "" {
		return linkId
	}
	return fallbackId
}
