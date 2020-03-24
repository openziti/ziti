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

package xlink_transwarp

import (
	"fmt"
	"github.com/netfoundry/ziti-fabric/router/xlink"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/sirupsen/logrus"
	"net"
)

func (self *dialer) Dial(addressString string, linkId *identity.TokenId) error {
	if address, err := net.ResolveUDPAddr("udp", addressString); err == nil {
		name := "l/" + linkId.Token
		logrus.Infof("dialing link [%s] at [%s]", name, address)

		if conn, err := net.ListenUDP("udp", self.config.bindAddress); err == nil {
			if err := writeHello(linkId, conn, address); err == nil {
				if peerId, peer, err := readHello(conn); err == nil {
					logrus.Infof("received hello [%s] from [%s], success", peerId.Token, peer)
				} else {
					return fmt.Errorf("error receiving hello from peer [%s] (%w)", address, err)
				}
			} else {
				return fmt.Errorf("error sending hello to peer [%s] (%w)", address, err)
			}

			if err := self.accepter.Accept(&impl{id: linkId, conn: conn}); err != nil {
				return fmt.Errorf("error accepting outgoing Xlink (%w)", err)
			}

			return nil

		} else {
			return fmt.Errorf("error dialing link [%s] (%w)", name, err)
		}
	} else {
		return fmt.Errorf("error parsing link address [%s] (%w)", addressString, err)
	}
}

type dialer struct {
	id       *identity.TokenId
	config   *dialerConfig
	accepter xlink.Accepter
}
