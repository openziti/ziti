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

package xlink

import (
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
)

type Factory interface {
	CreateListener(id *identity.TokenId, f Forwarder, config transport.Configuration) (Listener, error)
	CreateDialer(id *identity.TokenId, f Forwarder, config transport.Configuration) (Dialer, error)
}

type Listener interface {
	Listen() error
	GetAdvertisement() string
	Close() error
}

type Accepter interface {
	Accept(xlink Xlink) error
}

type Dialer interface {
	Dial(address string, id *identity.TokenId, routerId string) error
}

type Xlink interface {
	Id() *identity.TokenId
	SendPayload(payload *xgress.Payload) error
	SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error
	Close() error
	DestinationId() string
}

type Forwarder interface {
	ForwardPayload(srcAddr xgress.Address, payload *xgress.Payload) error
	ForwardAcknowledgement(srcAddr xgress.Address, acknowledgement *xgress.Acknowledgement) error
}
