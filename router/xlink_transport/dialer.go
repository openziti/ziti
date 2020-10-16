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
	"fmt"
	"github.com/google/uuid"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/sirupsen/logrus"
)

func (self *dialer) Dial(addressString string, linkId *identity.TokenId) error {
	address, err := transport.ParseAddress(addressString)
	if err != nil {
		return fmt.Errorf("error parsing link address [%s] (%w)", addressString, err)
	}
	logrus.Infof("dialing link with split payload/ack channels [l/%s]", linkId.Token)

	connId := uuid.New().String()

	payloadDialer := channel2.NewClassicDialer(linkId, address, map[int32][]byte{
		LinkHeaderConnId: []byte(connId),
		LinkHeaderType:   {PayloadChannel},
	})

	logrus.Infof("dialing payload channel for link [l/%s]", linkId.Token)

	payloadCh, err := channel2.NewChannelWithTransportConfiguration("l/"+linkId.Token, payloadDialer, self.config.options, self.tcfg)
	if err != nil {
		return fmt.Errorf("error dialing link [l/%s] for payload (%w)", linkId.Token, err)
	}

	ackDialer := channel2.NewClassicDialer(linkId, address, map[int32][]byte{
		LinkHeaderConnId: []byte(connId),
		LinkHeaderType:   {AckChannel},
	})

	logrus.Infof("dialing ack channel for link [l/%s]", linkId.Token)

	ackCh, err := channel2.NewChannelWithTransportConfiguration("l/"+linkId.Token, ackDialer, self.config.options, self.tcfg)
	if err != nil {
		_ = payloadCh.Close()
		return fmt.Errorf("error dialing link [l/%s] for ack (%w)", linkId.Token, err)
	}

	xlink := &splitImpl{id: linkId, payloadCh: payloadCh, ackCh: ackCh}

	if self.chAccepter != nil {
		if err := self.chAccepter.AcceptChannel(xlink, payloadCh, true); err != nil {
			_ = xlink.Close()
			return fmt.Errorf("error accepting outgoing channel (%w)", err)
		}

		if err := self.chAccepter.AcceptChannel(xlink, ackCh, false); err != nil {
			_ = xlink.Close()
			return fmt.Errorf("error accepting outgoing channel (%w)", err)
		}
	}

	if err := self.accepter.Accept(xlink); err != nil {
		return fmt.Errorf("error accepting outgoing Xlink (%w)", err)
	}

	return nil

}

type dialer struct {
	id         *identity.TokenId
	config     *dialerConfig
	accepter   xlink.Accepter
	chAccepter ChannelAccepter
	tcfg       transport.Configuration
}
