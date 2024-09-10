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
	"github.com/openziti/channel/v3"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/transport/v2"
	fabricMetrics "github.com/openziti/ziti/common/metrics"
	"github.com/openziti/ziti/router/xlink"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"sync"
	"time"
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
	metricsRegistry    metrics.Registry
	xlinkRegistery     xlink.Registry
}

func (self *listener) Listen() error {
	config := channel.ListenerConfig{
		ConnectOptions:     self.config.options.ConnectOptions,
		TransportConfig:    self.tcfg,
		PoolConfigurator:   fabricMetrics.GoroutinesPoolMetricsConfigF(self.metricsRegistry, "pool.listener.link"),
		ConnectionHandlers: []channel.ConnectionHandler{&ConnectionHandler{self.id}},
	}

	var err error
	if self.listener, err = channel.NewClassicListenerF(self.id, self.config.bind, config, self.acceptNewUnderlay); err != nil {
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

func (self *listener) Close() error {
	return self.listener.Close()
}

func (self *listener) GetLocalBinding() string {
	return self.config.bindInterface
}

func (self *listener) acceptNewUnderlay(underlay channel.Underlay) {
	if _, err := channel.NewChannelWithUnderlay("link", underlay, self, self.config.options); err != nil {
		logrus.WithError(err).Error("error creating link channel")
	}
}

func (self *listener) BindChannel(binding channel.Binding) error {
	log := pfxlog.ChannelLogger("link", "linkListener").
		WithField("linkProtocol", self.GetLinkProtocol()).
		WithField("linkId", binding.GetChannel().Id())

	headers := channel.Headers(binding.GetChannel().Underlay().Headers())
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
				id:            binding.GetChannel().Id(),
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
			return errors.Errorf("got two payload channels for link %v", binding.GetChannel().Id())
		}); err != nil {
			return false, nil, err
		}
	} else if chanType == AckChannel {
		if err := link.syncInit(func() error {
			if link.ackCh == nil {
				link.ackCh = binding.GetChannel()
				return nil
			}
			return errors.Errorf("got two ack channels for link %v", binding.GetChannel().Id())
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
		id:            binding.GetChannel().Id(),
		key:           self.xlinkRegistery.GetLinkKey(linkMeta.dialerBinding, self.GetLinkProtocol(), linkMeta.routerId, self.config.bindInterface),
		ch:            binding.GetChannel(),
		routerId:      linkMeta.routerId,
		routerVersion: linkMeta.routerVersion,
		linkProtocol:  self.GetLinkProtocol(),
		dialAddress:   self.GetAdvertisement(),
		iteration:     linkMeta.iteration,
		dialed:        false,
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
	iteration     uint32
}
