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
 * xlink.Dialer
 */
func (self *dialer) Dial(addressString string, linkId *identity.TokenId) error {
	if address, err := net.ResolveUDPAddr("udp", addressString); err == nil {
		name := "l/" + linkId.Token
		logrus.Infof("dialing link [%s] at [%s]", name, address)

		if conn, err := net.ListenUDP("udp", self.config.bindAddress); err == nil {
			waitCh := make(chan struct{}, 0)
			self.waiters[linkId.Token] = waitCh
			if err := writeHello(linkId, conn, address); err == nil {
				if m, peer, err := readMessage(conn); err == nil {
					if err := handleHello(m, conn, peer, self); err == nil {
						select {
						case <-waitCh:
							xli := newImpl(linkId, conn, address)
							if err := self.accepter.Accept(xli); err != nil {
								return fmt.Errorf("error accepting outgoing Xlink (%w)", err)
							}
							return nil

						case <-time.After(5 * time.Second):
							delete(self.waiters, linkId.Token)
							return fmt.Errorf("timeout in hello response")
						}
					} else {
						return fmt.Errorf("error handling hello from [%s] (%w)", peer, err)
					}
				} else {
					return fmt.Errorf("error receiving hello from peer [%s] (%w)", peer, err)
				}
			} else {
				return fmt.Errorf("error sending hello to peer from [%s] (%w)", address, err)
			}

		} else {
			return fmt.Errorf("error dialing link [%s] (%w)", name, err)
		}
	} else {
		return fmt.Errorf("error parsing link address [%s] (%w)", addressString, err)
	}
}

/*
 * xlink_transwarp.HelloHandler
 */
func (self *dialer) HandleHello(linkId *identity.TokenId, _ *net.UDPConn, peer *net.UDPAddr) {
	if ch, found := self.waiters[linkId.Token]; found {
		logrus.Infof("received hello [%s] from peer [%s], success", linkId.Token, peer)
		close(ch)
	} else {
		logrus.Errorf("invalid hello [%s] from peer [%s], failure", linkId.Token, peer)
	}
}

/*
 * xlink_transwarp.dialer
 */
type dialer struct {
	id       *identity.TokenId
	config   *dialerConfig
	accepter xlink.Accepter
	waiters  map[string]chan struct{}
}
