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
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/pkg/errors"
)

func (self *splitImpl) Id() *identity.TokenId {
	return self.id
}

func (self *splitImpl) SendPayload(payload *xgress.Payload) error {
	return self.payloadCh.Send(payload.Marshall())
}

func (self *splitImpl) SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error {
	return self.ackCh.Send(acknowledgement.Marshall())
}

func (self *splitImpl) Close() error {
	err := self.payloadCh.Close()
	err2 := self.ackCh.Close()
	if err == nil {
		return err2
	}
	if err2 == nil {
		return err
	}
	return errors.Errorf("multiple failures while closing transport link (%w) (%w)", err, err2)
}

type splitImpl struct {
	id        *identity.TokenId
	payloadCh channel2.Channel
	ackCh     channel2.Channel
}
