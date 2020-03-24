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

func (self *listener) Listen() error {
	listener, err := net.ListenUDP("udp", self.config.bindAddress)
	if err != nil {
		return fmt.Errorf("error listening (%w)", err)
	}
	self.listener = listener
	go self.listen()
	return nil
}

func (self *listener) GetAdvertisement() string {
	return self.config.advertiseAddress.String()
}

func (self *listener) listen() {
	for {
		linkId, peer, err := readHello(self.listener)
		if err == nil {
			logrus.Infof("read hello [%s] from peer at [%s], responding", linkId.Token, peer)
			if err := writeHello(linkId, self.listener, peer); err != nil {
				logrus.Errorf("error sending hello [%s] to peer at [%s] (%v)", linkId.Token, peer, err)
			}
		} else {
			logrus.Errorf("error reading hello from peer at [%s] (%v)", peer, err)
		}
	}
}

type listener struct {
	id       *identity.TokenId
	config   *listenerConfig
	listener *net.UDPConn
	accepter xlink.Accepter
}
