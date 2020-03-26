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
	"time"
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
	go self.ping()
	return nil
}

func (self *listener) GetAdvertisement() string {
	return self.config.advertiseAddress.String()
}

/*
 * xlink.Xlink
 */
func (self *listener) HandleHello(linkId *identity.TokenId, conn *net.UDPConn, peer *net.UDPAddr) {
	xlinkImpl := newImpl(linkId, conn, peer)
	if err := self.accepter.Accept(xlinkImpl); err == nil {
		self.peers[peer.String()] = xlinkImpl

		if err := writeHello(linkId, self.listener, peer); err == nil {
			logrus.Infof("sent hello [%s] to peer [%s]", linkId.Token, peer)
		} else {
			logrus.Errorf("error sending hello [%s] to peer at [%s] (%v)", linkId.Token, peer, err)
		}
	}
}

/*
 * listener
 */
func (self *listener) listen() {
	for {
		if err := readMessage(self.listener, self); err != nil {
			if nerr, ok := err.(net.Error); ok && !nerr.Timeout() {
				logrus.Errorf("error reading message (%v)", err)
			}
		}
	}
}

func (self *listener) ping() {
	for {
		time.Sleep(pingCycleDelayMs * time.Millisecond)

		for _, xlinkImpl := range self.peers {
			/*
			 * Send
			 */
			if time.Since(xlinkImpl.lastPingTx).Milliseconds() <= time.Since(xlinkImpl.lastPingRx).Milliseconds() && time.Since(xlinkImpl.lastPingTx).Milliseconds() >= pingDelayMs {
				if err := xlinkImpl.sendPing(); err == nil {
					logrus.Infof("sent ping for Xlink [l/%s]", xlinkImpl.id.Token)
				} else {
					logrus.Errorf("error sending ping for Xlink [l/%s] (%v)", xlinkImpl.id.Token, err)
				}
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

const pingDelayMs = 2000
const pingCycleDelayMs = 1000
