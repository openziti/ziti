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
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/sirupsen/logrus"
)

func (self *dialer) Dial(addressString string, linkId *identity.TokenId) error {
	if address, err := transport.ParseAddress(addressString); err == nil {
		logrus.Infof("dialing link [l/%s]", linkId.Token)

		dialer := channel2.NewClassicDialer(linkId, address, nil)
		ch, err := channel2.NewChannel("l/"+linkId.Token, dialer, self.config.options)
		if err == nil {
			xlink := &impl{id: linkId, ch: ch}

			if self.chAccepter != nil {
				if err := self.chAccepter.AcceptChannel(xlink, ch); err != nil {
					return fmt.Errorf("error accepting outgoing channel (%w)", err)
				}
			}

			if err := self.accepter.Accept(xlink); err != nil {
				return fmt.Errorf("error accepting outgoing Xlink (%w)", err)
			}

			return nil

		} else {
			return fmt.Errorf("error dialing link [l/%s] (%w)", linkId.Token, err)
		}
	} else {
		return fmt.Errorf("error parsing link address [%s] (%w)", addressString, err)
	}
}

type dialer struct {
	id         *identity.TokenId
	config     *dialerConfig
	accepter   xlink.Accepter
	chAccepter ChannelAccepter
}
