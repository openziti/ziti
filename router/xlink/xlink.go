/*
	(c) Copyright NetFoundry Inc.

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
	"fmt"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/inspect"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/transport/v2"
	"time"
)

// Registry contains known link instances and manages link de-duplication
type Registry interface {
	// UpdateLinkDest adds or updates the state of the given destination
	UpdateLinkDest(id string, version string, healthy bool, listeners []*ctrl_pb.Listener)

	// RemoveLinkDest removes the given link destination
	RemoveLinkDest(id string)

	// GetLink returns the link to the given router, of the given type, if one exists
	GetLink(linkKey string) (Xlink, bool)
	// GetLinkById returns the link for the given id, if it exists
	GetLinkById(linkId string) (Xlink, bool)
	// DialSucceeded notifies the registry that a dial succeed and provides the resulting link
	DialSucceeded(link Xlink) (Xlink, bool)
	// LinkAccepted notifies the registry that a link listener accepted a dial and provides the resulting link
	LinkAccepted(link Xlink) (Xlink, bool)
	// LinkClosed notifies the registry that a link closed
	LinkClosed(link Xlink)
	// Iter provides a channel which returns all known links
	Iter() <-chan Xlink
	// Shutdown frees any resources owned by the registry
	Shutdown()

	// SendRouterLinkMessage will notify the given controllers about the existing link
	SendRouterLinkMessage(link Xlink, channels ...channel.Channel)

	// Inspect will return debug information about the state of links and the registry
	Inspect(timeout time.Duration) *inspect.LinksInspectResult

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
	GetGroups() []string
	GetLocalBinding() string
	Close() error
}

type Acceptor interface {
	Accept(xlink Xlink) error
}

// A Dial contains the information need to dial another router
type Dial interface {
	GetLinkKey() string
	GetLinkId() string
	GetRouterId() string
	GetAddress() string
	GetLinkProtocol() string
	GetRouterVersion() string
}

type BackoffConfig interface {
	GetMinRetryInterval() time.Duration
	GetMaxRetryInterval() time.Duration
	GetRetryBackoffFactor() float64
}

type Dialer interface {
	Dial(dial Dial) (Xlink, error)
	GetGroups() []string
	GetBinding() string
	GetHealthyBackoffConfig() BackoffConfig
	GetUnhealthyBackoffConfig() BackoffConfig
	AdoptBinding(listener Listener)
}

type LinkDestination interface {
	Id() string
	SendPayload(payload *xgress.Payload) error
	SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error
	SendControl(control *xgress.Control) error
	InspectCircuit(circuitDetail *inspect.CircuitInspectDetail)
}

type Xlink interface {
	LinkDestination
	Key() string
	Init(metricsRegistry metrics.Registry) error
	Close() error
	CloseNotified() error
	DestinationId() string
	DestVersion() string
	LinkProtocol() string
	DialAddress() string
	HandleCloseNotification(f func())
	IsClosed() bool
	InspectLink() *inspect.LinkInspectDetail
	GetAddresses() []*ctrl_pb.LinkConn
	IsDialed() bool
}

type Forwarder interface {
	ForwardPayload(srcAddr xgress.Address, payload *xgress.Payload) error
	ForwardAcknowledgement(srcAddr xgress.Address, acknowledgement *xgress.Acknowledgement) error
	ForwardControl(srcAddr xgress.Address, control *xgress.Control) error
}

func GetLinkKey(dialerBinding, protocol, dest, listenerBinding string) string {
	if dialerBinding == "" {
		dialerBinding = "default"
	}
	if listenerBinding == "" {
		listenerBinding = "default"
	}
	return fmt.Sprintf("%s->%s:%s->%s", dialerBinding, protocol, dest, listenerBinding)
}
