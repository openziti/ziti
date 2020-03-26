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

/*
 * xlink.Listener
 */
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

/*
 * xlink_transwarp.HelloHandler
 */
func (self *listener) HandleHello(linkId *identity.TokenId, conn *net.UDPConn, peer *net.UDPAddr) {
	xlinkImpl := newImpl(linkId, conn, peer)
	if err := self.accepter.Accept(xlinkImpl); err == nil {
		self.peers[peer.String()] = xlinkImpl
		if err := writeHello(linkId, self.listener, peer); err == nil {
			go xlinkImpl.pinger()
			logrus.Infof("[hello->l/%s] -> %s", linkId.Token, peer)
		} else {
			logrus.Errorf("error sending hello for [l/%s] to peer at [%s] (%v)", linkId.Token, peer, err)
		}
	}
}

/*
 * xlink_transwarp.listener
 */
func (self *listener) listen() {
	for {
		if m, peer, err := readMessage(self.listener); err == nil {
			if m.messageType == Hello {
				if err := handleHello(m, self.listener, peer, self); err != nil {
					logrus.Errorf("error handling hello from [%s] (%v)", peer, err)
				}
			} else {
				if xlinkImpl, found := self.peers[peer.String()]; found {
					if err := handleMessage(m, self.listener, peer, xlinkImpl); err != nil {
						logrus.Errorf("error handling message from [%s] (%v)", peer, err)
					}
				}
			}
		} else {
			if nerr, ok := err.(net.Error); ok && !nerr.Timeout() {
				logrus.Errorf("error reading message (%v)", err)
			}
		}
	}
}

type listener struct {
	id       *identity.TokenId
	config   *listenerConfig
	listener *net.UDPConn
	accepter xlink.Accepter
	peers    map[string]*impl
}
