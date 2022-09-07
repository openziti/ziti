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
	"github.com/openziti/channel"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/transport/v2"
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
}

func (self *dialer) Dial(dial xlink.Dial) (xlink.Xlink, error) {
	address, err := transport.ParseAddress(dial.GetAddress())
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing link address [%s]", dial.GetAddress())
	}

	linkId := self.id.ShallowCloneWithNewToken(dial.GetLinkId())
	connId := uuid.New().String()

	var xli xlink.Xlink
	if self.config.split {
		xli, err = self.dialSplit(linkId, address, connId, dial)
	} else {
		xli, err = self.dialSingle(linkId, address, connId, dial)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "error dialing outgoing link [l/%s]", linkId.Token)
	}

	if err := self.acceptor.Accept(xli); err != nil {
		return nil, errors.Wrapf(err, "error accepting link [l/%s]", linkId.Token)
	}

	return xli, nil

}

func (self *dialer) dialSplit(linkId *identity.TokenId, address transport.Address, connId string, dial xlink.Dial) (xlink.Xlink, error) {
	logrus.Debugf("dialing link with split payload/ack channels [l/%s]", linkId.Token)

	payloadDialer := channel.NewClassicDialerWithBindAddress(linkId, address, self.config.localBinding, map[int32][]byte{
		LinkHeaderRouterId:      []byte(self.id.Token),
		LinkHeaderConnId:        []byte(connId),
		LinkHeaderType:          {byte(PayloadChannel)},
		LinkHeaderRouterVersion: []byte(dial.GetRouterVersion()),
	})

	logrus.Debugf("dialing payload channel for [l/%s]", linkId.Token)

	bindHandler := &splitDialBindHandler{
		dialer: self,
		link: &splitImpl{
			id:              linkId,
			routerId:        dial.GetRouterId(),
			routerVersion:   dial.GetRouterVersion(),
			linkProtocol:    dial.GetLinkProtocol(),
			droppedMsgMeter: self.metricsRegistry.Meter("link.dropped_msgs:" + linkId.Token),
		},
	}

	payloadCh, err := channel.NewChannelWithTransportConfiguration("l/"+linkId.Token, payloadDialer, channel.BindHandlerF(bindHandler.bindPayloadChannel), self.config.options, self.transportConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "error dialing payload channel for [l/%s]", linkId.Token)
	}

	logrus.Debugf("dialing ack channel for [l/%s]", linkId.Token)

	ackDialer := channel.NewClassicDialerWithBindAddress(linkId, address, self.config.localBinding, map[int32][]byte{
		LinkHeaderRouterId:      []byte(self.id.Token),
		LinkHeaderConnId:        []byte(connId),
		LinkHeaderType:          {byte(AckChannel)},
		LinkHeaderRouterVersion: []byte(dial.GetRouterVersion()),
	})

	_, err = channel.NewChannelWithTransportConfiguration("l/"+linkId.Token, ackDialer, channel.BindHandlerF(bindHandler.bindAckChannel), self.config.options, self.transportConfig)
	if err != nil {
		_ = payloadCh.Close()
		return nil, errors.Wrapf(err, "error dialing ack channel for [l/%s]", linkId.Token)
	}

	return bindHandler.link, nil
}

func (self *dialer) dialSingle(linkId *identity.TokenId, address transport.Address, connId string, dial xlink.Dial) (xlink.Xlink, error) {
	logrus.Debugf("dialing link with single channel [l/%s]", linkId.Token)

	payloadDialer := channel.NewClassicDialerWithBindAddress(linkId, address, self.config.localBinding, map[int32][]byte{
		LinkHeaderRouterId:      []byte(self.id.Token),
		LinkHeaderConnId:        []byte(connId),
		LinkHeaderRouterVersion: []byte(dial.GetRouterVersion()),
	})

	bindHandler := &dialBindHandler{
		dialer: self,
		link: &impl{
			id:              linkId,
			routerId:        dial.GetRouterId(),
			linkProtocol:    dial.GetLinkProtocol(),
			routerVersion:   dial.GetRouterVersion(),
			droppedMsgMeter: self.metricsRegistry.Meter("link.dropped_msgs:" + linkId.Token),
		},
	}

	_, err := channel.NewChannelWithTransportConfiguration("l/"+linkId.Token, payloadDialer, bindHandler, self.config.options, self.transportConfig)
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
	self.link.payloadCh = binding.GetChannel()
	bindHandler := self.dialer.bindHandlerFactory.NewBindHandler(self.link, true, false)
	if err := bindHandler.BindChannel(binding); err != nil {
		return errors.Wrapf(err, "error accepting outgoing payload channel for [l/%s]", self.link.id.Token)
	}
	return nil
}

func (self *splitDialBindHandler) bindAckChannel(binding channel.Binding) error {
	self.link.ackCh = binding.GetChannel()
	bindHandler := self.dialer.bindHandlerFactory.NewBindHandler(self.link, false, false)
	if err := bindHandler.BindChannel(binding); err != nil {
		return errors.Wrapf(err, "error accepting outgoing ack channel for [l/%s]", self.link.id.Token)
	}
	return nil
}
