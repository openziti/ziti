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
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/transport/v2"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/router/xlink"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type dialer struct {
	id                 *identity.TokenId
	config             *dialerConfig
	bindHandlerFactory BindHandlerFactory
	acceptor           xlink.Acceptor
	transportConfig    transport.Configuration
	metricsRegistry    metrics.Registry
	adoptedBinding     string
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
	address, err := transport.ParseAddress(dial.GetAddress())
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing link address [%s]", dial.GetAddress())
	}

	linkId := self.id.ShallowCloneWithNewToken(dial.GetLinkId())
	connId := uuid.NewString()

	var xli xlink.Xlink
	if self.config.split {
		xli, err = self.dialSplit(linkId, address, connId, dial)
	} else {
		xli, err = self.dialSingle(linkId, address, connId, dial)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "error dialing outgoing link [l/%s@%v]", linkId.Token, dial.GetIteration())
	}

	if err = self.acceptor.Accept(xli); err != nil {
		if closeErr := xli.Close(); closeErr != nil {
			pfxlog.Logger().WithError(closeErr).WithField("acceptErr", err).Error("error closing link after accept error")
		}
		return nil, errors.Wrapf(err, "error accepting link [l/%s@%v]", linkId.Token, dial.GetIteration())
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

type dialBindHandler struct {
	dialer *dialer
	link   *impl
}

func (self *dialBindHandler) BindChannel(binding channel.Binding) error {
	self.link.ch = binding.GetChannel()
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
