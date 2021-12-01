/*
	(c) Copyright NetFoundry, Inc.

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
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (self *dialer) Dial(addressString string, linkId *identity.TokenId, routerId string) error {
	address, err := transport.ParseAddress(addressString)
	if err != nil {
		return errors.Wrapf(err, "error parsing link address [%s]", addressString)
	}

	connId := uuid.New().String()

	var xli xlink.Xlink
	if self.config.split {
		xli, err = self.dialSplit(linkId, address, routerId, connId)
	} else {
		xli, err = self.dialSingle(linkId, address, routerId, connId)
	}
	if err != nil {
		return errors.Wrapf(err, "error dialing outgoing link [l/%s]", linkId.Token)
	}

	if err := self.accepter.Accept(xli); err != nil {
		return errors.Wrapf(err, "error accepting link [l/%s]", linkId.Token)
	}

	return nil

}

func (self *dialer) dialSplit(linkId *identity.TokenId, address transport.Address, routerId, connId string) (xlink.Xlink, error) {
	logrus.Debugf("dialing link with split payload/ack channels [l/%s]", linkId.Token)

	payloadDialer := channel2.NewClassicDialer(linkId, address, map[int32][]byte{
		LinkHeaderRouterId: []byte(routerId),
		LinkHeaderConnId:   []byte(connId),
		LinkHeaderType:     {PayloadChannel},
	})

	logrus.Debugf("dialing payload channel for [l/%s]", linkId.Token)

	payloadCh, err := channel2.NewChannelWithTransportConfiguration("l/"+linkId.Token, payloadDialer, self.config.options, self.tcfg)
	if err != nil {
		return nil, errors.Wrapf(err, "error dialing payload channel for [l/%s]", linkId.Token)
	}

	ackDialer := channel2.NewClassicDialer(linkId, address, map[int32][]byte{
		LinkHeaderConnId: []byte(connId),
		LinkHeaderType:   {AckChannel},
	})

	logrus.Debugf("dialing ack channel for [l/%s]", linkId.Token)

	ackCh, err := channel2.NewChannelWithTransportConfiguration("l/"+linkId.Token, ackDialer, self.config.options, self.tcfg)
	if err != nil {
		_ = payloadCh.Close()
		return nil, errors.Wrapf(err, "error dialing ack channel for [l/%s]", linkId.Token)
	}

	xli := &splitImpl{id: linkId, payloadCh: payloadCh, ackCh: ackCh}

	if self.chAccepter != nil {
		if err := self.chAccepter.AcceptChannel(xli, payloadCh, true, false); err != nil {
			_ = xli.Close()
			return nil, errors.Wrapf(err, "error accepting outgoing payload channel for [l/%s]", linkId.Token)
		}
		if err := self.chAccepter.AcceptChannel(xli, ackCh, false, false); err != nil {
			_ = xli.Close()
			return nil, errors.Wrapf(err, "error accepting outgoing ack channel for [l/%s]", linkId.Token)
		}
	}

	return xli, nil
}

func (self *dialer) dialSingle(linkId *identity.TokenId, address transport.Address, routerId, connId string) (xlink.Xlink, error) {
	logrus.Debugf("dialing link with single channel [l/%s]", linkId.Token)

	payloadDialer := channel2.NewClassicDialer(linkId, address, map[int32][]byte{
		LinkHeaderRouterId: []byte(routerId),
		LinkHeaderConnId:   []byte(connId),
	})

	payloadCh, err := channel2.NewChannelWithTransportConfiguration("l/"+linkId.Token, payloadDialer, self.config.options, self.tcfg)
	if err != nil {
		return nil, errors.Wrapf(err, "dialing link [l/%s] for payload", linkId.Token)
	}

	xli := &impl{
		id:       linkId,
		ch:       payloadCh,
		routerId: routerId,
	}

	if self.chAccepter != nil {
		if err := self.chAccepter.AcceptChannel(xli, payloadCh, true, false); err != nil {
			_ = xli.Close()
			return nil, errors.Wrapf(err, "error accepting outgoing channel for [l/%s]", linkId.Token)
		}
	}

	return xli, nil
}

type dialer struct {
	id         *identity.TokenId
	config     *dialerConfig
	accepter   xlink.Accepter
	chAccepter ChannelAccepter
	tcfg       transport.Configuration
}
