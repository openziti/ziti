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
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"net"
	"sync"
	"time"
)

/*
 * xlink.Xlink
 */
func (self *impl) Id() *identity.TokenId {
	return self.id
}

func (self *impl) SendPayload(payload *xgress.Payload) error {
	return fmt.Errorf("not implemented")
}

func (self *impl) SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error {
	return fmt.Errorf("not implemented")
}

func (self *impl) Close() error {
	return self.conn.Close()
}

/*
 * impl
 */
func (self *impl) sendPing() error {
	sequence := self.nextSequence()
	if err := writePing(sequence, self.conn, self.peer, noReplyFor); err != nil {
		return fmt.Errorf("error sending ping (%w)", err)
	}
	self.lastPingTxSequence = sequence
	self.lastPingTx = time.Now()
	return nil
}

func (self *impl) nextSequence() int32 {
	self.sequenceLock.Lock()
	defer self.sequenceLock.Unlock()

	sequence := self.sequence
	self.sequence++
	return sequence
}

func newImpl(id *identity.TokenId, conn *net.UDPConn, peer *net.UDPAddr) *impl {
	return &impl{id: id, conn: conn, peer: peer}
}

type impl struct {
	id                 *identity.TokenId
	conn               *net.UDPConn
	peer               *net.UDPAddr
	sequence           int32
	sequenceLock       sync.Mutex
	lastPingRx         time.Time
	lastPingTx         time.Time
	lastPingTxSequence int32
}
