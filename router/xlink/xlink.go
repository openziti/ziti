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
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/openziti/fabric/inspect"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/transport"
)

// Registry contains known link instances and manages link de-duplication
type Registry interface {
	xctrl.Xctrl

	// GetLink returns the link to the given router, of the given type, if one exists
	GetLink(routerId, linkType string) (Xlink, bool)
	// GetLinkById returns the link for the given id, if it exists
	GetLinkById(linkId string) (Xlink, bool)
	// GetDialLock tries to acquire a dial lock for the given dial attempt
	GetDialLock(dial Dial) (Xlink, bool)
	// DialFailed notifies the registry that a dial failed
	DialFailed(dial Dial)
	// DialSucceeded notifies the registry that a dial succeed and provides the resulting link
	DialSucceeded(link Xlink) (Xlink, bool)
	// LinkAccepted notifes the registry that a link listener accepted a dial and provides the resulting link
	LinkAccepted(link Xlink) (Xlink, bool)
	// LinkClosed notifies the registry that a link closed
	LinkClosed(link Xlink)
	// Iter provides a channel which returns all known links
	Iter() <-chan Xlink
	// Shutdown frees any resources owned by the registry
	Shutdown()

	// DebugForgetLink will remove the link from the registry to inject an error condition
	DebugForgetLink(linkId string) bool
}

// A Factory creates link listeners and link dialers
type Factory interface {
	CreateListener(id *identity.TokenId, f Forwarder, config transport.Configuration) (Listener, error)
	CreateDialer(id *identity.TokenId, f Forwarder, config transport.Configuration) (Dialer, error)
}

type Listener interface {
	Listen() error
	GetAdvertisement() string
	GetLinkProtocol() string
	GetLinkCostTags() []string
	Close() error
}

type Acceptor interface {
	Accept(xlink Xlink) error
}

// A Dial contains the information need to dial another router
type Dial interface {
	GetLinkId() string
	GetRouterId() string
	GetAddress() string
	GetLinkProtocol() string
	GetRouterVersion() string
}

type Dialer interface {
	Dial(dial Dial) (Xlink, error)
}

type Xlink interface {
	Id() *identity.TokenId
	SendPayload(payload *xgress.Payload) error
	SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error
	SendControl(control *xgress.Control) error
	Close() error
	CloseNotified() error
	DestinationId() string
	DestVersion() string
	LinkProtocol() string
	HandleCloseNotification(f func())
	IsClosed() bool
	InspectCircuit(circuitDetail *inspect.CircuitInspectDetail)
	InspectLink() *inspect.LinkInspectDetail
}

type Forwarder interface {
	ForwardPayload(srcAddr xgress.Address, payload *xgress.Payload) error
	ForwardAcknowledgement(srcAddr xgress.Address, acknowledgement *xgress.Acknowledgement) error
	ForwardControl(srcAddr xgress.Address, control *xgress.Control) error
}
