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
	"time"

	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/xlink"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var minMultiUnderlayVersion versions.SemVer
var devVersion versions.SemVer

func init() {
	minMultiUnderlayVersion = *versions.MustParseSemVer("1.6.6")
	devVersion = *versions.MustParseSemVer("0.0.0")
}

type dialer struct {
	id                 *identity.TokenId
	config             *dialerConfig
	bindHandlerFactory BindHandlerFactory
	acceptor           xlink.Acceptor
	transportConfig    transport.Configuration
	adoptedBinding     string
	env                LinkEnv
}

func (self *dialer) GetHealthyBackoffConfig() xlink.BackoffConfig {
	return self.config.healthyBackoffConfig
}

func (self *dialer) GetUnhealthyBackoffConfig() xlink.BackoffConfig {
	return self.config.unhealthyBackoffConfig
}

func (self *dialer) GetGroups() []string {
	return self.config.groups
}

func (self *dialer) AdoptBinding(l xlink.Listener) {
	self.adoptedBinding = l.GetLocalBinding()
}

func (self *dialer) GetBinding() string {
	if self.adoptedBinding != "" {
		return self.adoptedBinding
	}
	return self.config.localBinding
}

func (self *dialer) Dial(dial xlink.Dial) (xlink.Xlink, error) {
	log := pfxlog.Logger().WithField("linkId", dial.GetLinkId()).
		WithField("dialAddress", dial.GetAddress()).
		WithField("destRouterId", dial.GetRouterId()).
		WithField("destRouterVersion", dial.GetRouterVersion())

	address, err := transport.ParseAddress(dial.GetAddress())
	if err != nil {
		return nil, fmt.Errorf("error parsing link address [%s] (%w)", dial.GetAddress(), err)
	}

	linkId := self.id.ShallowCloneWithNewToken(dial.GetLinkId())
	connId := uuid.NewString()

	dialRouterVersion, err := versions.ParseSemVer(dial.GetRouterVersion())
	isDevVersion := err == nil && dialRouterVersion.Equals(&devVersion)
	supportsMultiUnderlay := err == nil && (dialRouterVersion.CompareTo(&minMultiUnderlayVersion) >= 0)

	var xli xlink.Xlink
	if isDevVersion || supportsMultiUnderlay {
		xli, err = self.dialMulti(linkId, address, connId, dial)
	} else if self.config.split {
		log.Info("destination router doesn't support multi-underlay links, falling back to split impl")
		xli, err = self.dialSplit(linkId, address, connId, dial)
	} else {
		log.Info("destination router doesn't support multi-underlay links, falling back to single impl")
		xli, err = self.dialSingle(linkId, address, connId, dial)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "error dialing outgoing link [l/%s@%v]", linkId.Token, dial.GetIteration())
	}

	if err = self.acceptor.Accept(xli); err != nil {
		if closeErr := xli.Close(); closeErr != nil {
			log.WithError(closeErr).WithField("acceptErr", err).Error("error closing link after accept error")
		}
		return nil, fmt.Errorf("error accepting link [l/%s@%v] (%w)", linkId.Token, dial.GetIteration(), err)
	}

	return xli, nil

}

func (self *dialer) dialSplit(linkId *identity.TokenId, address transport.Address, connId string, dial xlink.Dial) (xlink.Xlink, error) {
	log := pfxlog.Logger().WithFields(logrus.Fields{
		"linkId": linkId.Token,
		"connId": connId,
	})

	log.Info("dialing link with split payload/ack channels")

	headers := channel.Headers{
		LinkHeaderRouterId:      []byte(self.id.Token),
		LinkHeaderConnId:        []byte(connId),
		LinkHeaderType:          {byte(PayloadChannel)},
		LinkHeaderRouterVersion: []byte(dial.GetRouterVersion()),
		LinkHeaderBinding:       []byte(self.GetBinding()),
		LinkDialedRouterId:      []byte(dial.GetRouterId()),
	}
	headers.PutUint32Header(LinkHeaderIteration, dial.GetIteration())

	channelDialerConfig := channel.DialerConfig{
		Identity:        linkId,
		Endpoint:        address,
		LocalBinding:    self.config.localBinding,
		Headers:         headers,
		TransportConfig: self.transportConfig,
		MessageStrategy: channel.DatagramMessageStrategy(xgress.UnmarshallPacketPayload),
	}

	payloadDialer := channel.NewClassicDialer(channelDialerConfig)

	log.Info("dialing payload channel")

	bindHandler := &splitDialBindHandler{
		dialer: self,
		link: &splitImpl{
			id:            dial.GetLinkId(),
			key:           dial.GetLinkKey(),
			routerId:      dial.GetRouterId(),
			routerVersion: dial.GetRouterVersion(),
			linkProtocol:  dial.GetLinkProtocol(),
			dialAddress:   dial.GetAddress(),
			iteration:     dial.GetIteration(),
			dialed:        true,
		},
	}

	payloadCh, err := channel.NewChannel("l/"+linkId.Token, payloadDialer, channel.BindHandlerF(bindHandler.bindPayloadChannel), self.config.options)
	if err != nil {
		return nil, errors.Wrapf(err, "error dialing payload channel for [l/%s]", linkId.Token)
	}

	log.Info("dialing ack channel")

	headers.PutByteHeader(LinkHeaderType, byte(AckChannel))
	ackDialer := channel.NewClassicDialer(channelDialerConfig)

	_, err = channel.NewChannel("l/"+linkId.Token, ackDialer, channel.BindHandlerF(bindHandler.bindAckChannel), self.config.options)
	if err != nil {
		_ = payloadCh.Close()
		return nil, errors.Wrapf(err, "error dialing ack channel for [l/%s]", linkId.Token)
	}

	return bindHandler.link, nil
}

func (self *dialer) dialSingle(linkId *identity.TokenId, address transport.Address, connId string, dial xlink.Dial) (xlink.Xlink, error) {
	log := pfxlog.Logger().WithFields(logrus.Fields{
		"linkId": linkId.Token,
		"connId": connId,
	})

	log.Info("dialing link with single channel")

	headers := channel.Headers{
		LinkHeaderRouterId:      []byte(self.id.Token),
		LinkHeaderConnId:        []byte(connId),
		LinkHeaderRouterVersion: []byte(dial.GetRouterVersion()),
		LinkHeaderBinding:       []byte(self.GetBinding()),
		LinkDialedRouterId:      []byte(dial.GetRouterId()),
	}
	headers.PutUint32Header(LinkHeaderIteration, dial.GetIteration())

	payloadDialer := channel.NewClassicDialer(channel.DialerConfig{
		Identity:        linkId,
		Endpoint:        address,
		LocalBinding:    self.config.localBinding,
		Headers:         headers,
		TransportConfig: self.transportConfig,
		MessageStrategy: channel.DatagramMessageStrategy(xgress.UnmarshallPacketPayload),
	})

	bindHandler := &dialBindHandler{
		dialer: self,
		link: &impl{
			id:            dial.GetLinkId(),
			key:           dial.GetLinkKey(),
			routerId:      dial.GetRouterId(),
			linkProtocol:  dial.GetLinkProtocol(),
			routerVersion: dial.GetRouterVersion(),
			dialAddress:   dial.GetAddress(),
			iteration:     dial.GetIteration(),
			dialed:        true,
		},
	}

	_, err := channel.NewChannel("l/"+linkId.Token, payloadDialer, bindHandler, self.config.options)
	if err != nil {
		return nil, errors.Wrapf(err, "error dialing link [l/%s]", linkId.Token)
	}

	return bindHandler.link, nil
}

func (self *dialer) dialMulti(linkId *identity.TokenId, address transport.Address, connId string, dial xlink.Dial) (xlink.Xlink, error) {
	log := pfxlog.Logger().WithFields(logrus.Fields{
		"linkId": linkId.Token,
		"connId": connId,
	})

	log.Info("dialing link with multi-underlay channel")

	headers := channel.Headers{
		LinkHeaderRouterId:      []byte(self.id.Token),
		LinkHeaderConnId:        []byte(connId),
		LinkHeaderRouterVersion: []byte(dial.GetRouterVersion()),
		LinkHeaderBinding:       []byte(self.GetBinding()),
		LinkDialedRouterId:      []byte(dial.GetRouterId()),
	}
	headers.PutUint32Header(LinkHeaderIteration, dial.GetIteration())
	headers.PutBoolHeader(channel.IsGroupedHeader, true)
	headers.PutStringHeader(channel.TypeHeader, ChannelTypeDefault)
	headers.PutBoolHeader(channel.IsFirstGroupConnection, true)

	linkDialer := channel.NewClassicDialer(channel.DialerConfig{
		Identity:        linkId,
		Endpoint:        address,
		LocalBinding:    self.config.localBinding,
		Headers:         headers,
		TransportConfig: self.transportConfig,
		MessageStrategy: channel.DatagramMessageStrategy(xgress.UnmarshallPacketPayload),
	})

	bindHandler := &dialBindHandler{
		dialer: self,
		link: &impl{
			id:            dial.GetLinkId(),
			key:           dial.GetLinkKey(),
			routerId:      dial.GetRouterId(),
			linkProtocol:  dial.GetLinkProtocol(),
			routerVersion: dial.GetRouterVersion(),
			dialAddress:   dial.GetAddress(),
			iteration:     dial.GetIteration(),
			dialed:        true,
		},
	}

	underlay, err := linkDialer.CreateWithHeaders(self.config.options.ConnectTimeout, headers)
	if err != nil {
		return nil, fmt.Errorf("error dialing link [l/%s] (%w)", linkId.Token, err)
	}

	if isGrouped, _ := channel.Headers(underlay.Headers()).GetBoolHeader(channel.IsGroupedHeader); isGrouped {
		dialLinkChangeConfig := DialLinkChannelConfig{
			Dialer:             linkDialer,
			Underlay:           underlay,
			MaxDefaultChannels: int(self.config.maxDefaultConnections),
			MaxAckChannel:      int(self.config.maxAckConnections),
			StartupDelay:       self.config.startupDelay,
			UnderlayChangeCallback: func(ch *DialLinkChannel) {
				self.notifyOfLinkChange(ch, bindHandler.link)
			},
		}
		var dialLinkChannel = NewDialLinkChannel(dialLinkChangeConfig)
		multiChannelConfig := &channel.MultiChannelConfig{
			LogicalName:     fmt.Sprintf("l/%s", underlay.Id()),
			Options:         self.config.options,
			UnderlayHandler: dialLinkChannel,
			BindHandler:     bindHandler,
			Underlay:        underlay,
		}
		_, err = channel.NewMultiChannel(multiChannelConfig)
	} else {
		_, err = channel.NewChannelWithUnderlay(fmt.Sprintf("ziti-link[router=%v]", address.String()), underlay, bindHandler, self.config.options)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "error dialing link [l/%s]", linkId.Token)
	}

	return bindHandler.link, nil
}

func (self *dialer) notifyOfLinkChange(ch *DialLinkChannel, link xlink.Xlink) {
	if ch.GetChannel().IsClosed() { // don't send connection changes for closed links. close notification covers everything
		return
	}

	log := pfxlog.Logger().WithField("linkId", link.Id())

	stateUuid, first := ch.LinkConnectionsChanged(self.env.GetNetworkControllers())
	if first { // initial connection is reported by newRouterLink event
		return
	}

	message := &ctrl_pb.LinkStateUpdate{
		LinkId:        link.Id(),
		LinkIteration: link.Iteration(),
		ConnState:     link.GetLinkConnState(),
	}

	err := self.env.GetRateLimiterPool().QueueOrError(func() {
		channels := self.env.GetNetworkControllers().AllResponsiveCtrlChannels()

		if len(channels) == 0 {
			log.Info("no controllers available to notify of link")
			return
		}

		for _, ctrlCh := range channels {
			if err := protobufs.MarshalTyped(message).WithTimeout(time.Second).Send(ctrlCh); err != nil {
				log.WithError(err).Error("error sending router link state message")
			} else {
				log.WithField("ctrlId", ctrlCh.Id()).Info("notified controller of updated router link state")
				ch.MarkLinkStateSyncedForState(ctrlCh.Id(), stateUuid)
			}
		}
	})

	if err != nil {
		log.WithError(err).Error("failed to queue send of router link state message")
	}
}

type dialBindHandler struct {
	dialer *dialer
	link   *impl
}

func (self *dialBindHandler) BindChannel(binding channel.Binding) error {
	if mc, ok := binding.GetChannel().(channel.MultiChannel); ok {
		if linkChan, ok := mc.GetUnderlayHandler().(LinkChannel); ok {
			linkChan.InitChannel(mc)
			self.link.ch = linkChan
		}
	}

	if self.link.ch == nil {
		self.link.ch = NewSingleLinkChannel(binding.GetChannel())
	}

	bindHandler := self.dialer.bindHandlerFactory.NewBindHandler(self.link, true, false)
	return bindHandler.BindChannel(binding)
}

type splitDialBindHandler struct {
	link   *splitImpl
	dialer *dialer
}

func (self *splitDialBindHandler) bindPayloadChannel(binding channel.Binding) error {
	return self.link.syncInit(func() error {
		self.link.payloadCh = binding.GetChannel()
		bindHandler := self.dialer.bindHandlerFactory.NewBindHandler(self.link, true, false)
		if err := bindHandler.BindChannel(binding); err != nil {
			return errors.Wrapf(err, "error accepting outgoing payload channel for [l/%s]", self.link.id)
		}
		return nil
	})
}

func (self *splitDialBindHandler) bindAckChannel(binding channel.Binding) error {
	return self.link.syncInit(func() error {

		self.link.ackCh = binding.GetChannel()
		bindHandler := self.dialer.bindHandlerFactory.NewBindHandler(self.link, false, false)
		if err := bindHandler.BindChannel(binding); err != nil {
			return errors.Wrapf(err, "error accepting outgoing ack channel for [l/%s]", self.link.id)
		}
		return nil
	})
}
