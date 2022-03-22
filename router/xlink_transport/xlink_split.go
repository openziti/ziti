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
	"github.com/openziti/channel"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/pkg/errors"
)

type splitImpl struct {
	id            *identity.TokenId
	payloadCh     channel.Channel
	ackCh         channel.Channel
	routerId      string
	routerVersion string
	linkProtocol  string
	closeNotified concurrenz.AtomicBoolean
}

func (self *splitImpl) Id() *identity.TokenId {
	return self.id
}

func (self *splitImpl) SendPayload(payload *xgress.Payload) error {
	return self.payloadCh.Send(payload.Marshall())
}

func (self *splitImpl) SendControl(control *xgress.Control) error {
	return self.payloadCh.Send(control.Marshall())
}

func (self *splitImpl) SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error {
	return self.ackCh.Send(acknowledgement.Marshall())
}

func (self *splitImpl) CloseNotified() error {
	self.closeNotified.Set(true)
	return self.Close()
}

func (self *splitImpl) Close() error {
	var err, err2 error
	if self.payloadCh != nil {
		err = self.payloadCh.Close()
	}

	if self.ackCh != nil {
		err2 = self.ackCh.Close()
	}
	if err == nil {
		return err2
	}
	if err2 == nil {
		return err
	}
	return errors.Errorf("multiple failures while closing transport link (%v) (%v)", err, err2)
}

func (self *splitImpl) DestinationId() string {
	return self.routerId
}

func (self *splitImpl) DestVersion() string {
	return self.routerVersion
}

func (self *splitImpl) LinkProtocol() string {
	return self.linkProtocol
}

func (self *splitImpl) HandleCloseNotification(f func()) {
	if self.closeNotified.CompareAndSwap(false, true) {
		f()
	}
}
