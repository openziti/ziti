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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	fabricMetrics "github.com/openziti/fabric/metrics"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/transport/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

type listener struct {
	id                 *identity.TokenId
	config             *listenerConfig
	listener           channel.UnderlayListener
	accepter           xlink.Acceptor
	bindHandlerFactory BindHandlerFactory
	tcfg               transport.Configuration
	pendingLinks       map[string]*pendingLink
	lock               sync.Mutex
	metricsRegistry    metrics.Registry
	xlinkRegistery     xlink.Registry
}

func (self *listener) Listen() error {
	config := channel.ListenerConfig{
		ConnectOptions:   self.config.options.ConnectOptions,
		TransportConfig:  self.tcfg,
		PoolConfigurator: fabricMetrics.GoroutinesPoolMetricsConfigF(self.metricsRegistry, "pool.listener.link"),
	}
	listener := channel.NewClassicListener(self.id, self.config.bind, config)

	self.listener = listener
	connectionHandler := &ConnectionHandler{self.id}
	if err := self.listener.Listen(connectionHandler); err != nil {
		return fmt.Errorf("error listening (%w)", err)
	}
	go self.acceptLoop()
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

func (self *listener) Close() error {
	return self.listener.Close()
}

func (self *listener) GetLocalBinding() string {
	return self.config.bindInterface
}

func (self *listener) acceptLoop() {
	for {
		_, err := channel.NewChannelWithTransportConfiguration("link", self.listener, self, self.config.options, self.tcfg)
		if err != nil && errors.Is(err, channel.ListenerClosedError) {
			logrus.Errorf("link underlay acceptor closed")
			return
		} else if err != nil {
			logrus.Errorf("error creating link underlay (%v)", err)
			continue
		}
	}
}

func (self *listener) BindChannel(binding channel.Binding) error {
	log := pfxlog.ChannelLogger("link", "linkListener").
		WithField("linkProtocol", self.GetLinkProtocol()).
		WithField("linkId", binding.GetChannel().Id())

	headers := binding.GetChannel().Underlay().Headers()
	var chanType channelType

	routerId := ""
	routerVersion := ""
	dialerBinding := ""

	if headers != nil {
		if v, ok := headers[LinkHeaderRouterId]; ok {
			routerId = string(v)
			log = log.WithField("routerId", routerId)
			log.Info("accepting link")
		}
		if val, ok := headers[LinkHeaderType]; ok {
			chanType = channelType(val[0])
		}
		if val, ok := headers[LinkHeaderRouterVersion]; ok {
			routerVersion = string(val)
			log = log.WithField("routerVersion", routerVersion)
		}
		if val, ok := headers[LinkHeaderBinding]; ok {
			dialerBinding = string(val)
			log = log.WithField("dialerBinding", dialerBinding)
		}
	}

	linkMeta := &linkMetadata{
		routerId:      routerId,
		routerVersion: routerVersion,
		dialerBinding: dialerBinding,
	}

	if chanType != 0 {
		log = log.WithField("channelType", chanType)
		return self.bindSplitChannel(binding, chanType, linkMeta, log)
	}

	return self.bindNonSplitChannel(binding, linkMeta, log)
}

func (self *listener) bindSplitChannel(binding channel.Binding, chanType channelType, linkMeta *linkMetadata, log *logrus.Entry) error {
	headers := binding.GetChannel().Underlay().Headers()
	id, ok := headers[LinkHeaderConnId]
	if !ok {
		return errors.New("split conn received but missing connection id. closing")
	}

	log.Info("accepted part of split conn")

	xli, err := self.getOrCreateSplitLink(string(id), linkMeta, binding, chanType)
	if err != nil {
		log.WithError(err).Error("error binding link channel")
		return err
	}

	latencyPing := chanType == PayloadChannel
	if err := self.bindHandlerFactory.NewBindHandler(xli, latencyPing, true).BindChannel(binding); err != nil {
		return err
	}

	if xli.payloadCh != nil && xli.ackCh != nil {
		if err := self.accepter.Accept(xli); err != nil {
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

func (self *listener) getOrCreateSplitLink(id string, linkMeta *linkMetadata, binding channel.Binding, chanType channelType) (*splitImpl, error) {
	self.lock.Lock()
	defer self.lock.Unlock()

	var link *splitImpl

	if pending, found := self.pendingLinks[id]; found {
		delete(self.pendingLinks, id)
		link = pending.link
	} else {
		pending = &pendingLink{
			link: &splitImpl{
				id:            binding.GetChannel().Id(),
				key:           xlink.GetLinkKey(linkMeta.dialerBinding, self.GetLinkProtocol(), linkMeta.routerId, self.config.bindInterface),
				routerId:      linkMeta.routerId,
				routerVersion: linkMeta.routerVersion,
				linkProtocol:  self.GetLinkProtocol(),
				dialAddress:   self.GetAdvertisement(),
			},
			eventTime: time.Now(),
		}
		self.pendingLinks[id] = pending
		link = pending.link
	}

	if chanType == PayloadChannel {
		if link.payloadCh == nil {
			link.payloadCh = binding.GetChannel()
		} else {
			return nil, errors.Errorf("got two payload channels for link %v", binding.GetChannel().Id())
		}
	} else if chanType == AckChannel {
		if link.ackCh == nil {
			link.ackCh = binding.GetChannel()
		} else {
			return nil, errors.Errorf("got two ack channels for link %v", binding.GetChannel().Id())
		}
	} else {
		return nil, errors.Errorf("invalid channel type %v", chanType)
	}

	return link, nil
}

func (self *listener) bindNonSplitChannel(binding channel.Binding, linkMeta *linkMetadata, log *logrus.Entry) error {
	xli := &impl{
		id:            binding.GetChannel().Id(),
		key:           xlink.GetLinkKey(linkMeta.dialerBinding, self.GetLinkProtocol(), linkMeta.routerId, self.config.bindInterface),
		ch:            binding.GetChannel(),
		routerId:      linkMeta.routerId,
		routerVersion: linkMeta.routerVersion,
		linkProtocol:  self.GetLinkProtocol(),
		dialAddress:   self.GetAdvertisement(),
	}

	bindHandler := self.bindHandlerFactory.NewBindHandler(xli, true, true)
	if err := bindHandler.BindChannel(binding); err != nil {
		return errors.Wrapf(err, "error binding channel for link [l/%v]", binding.GetChannel().Id())
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

	for range ticker.C {
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
	}
}

type pendingLink struct {
	link      *splitImpl
	eventTime time.Time
}

type linkMetadata struct {
	routerId      string
	routerVersion string
	dialerBinding string
}
