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
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
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

	// GetLinkKey returns the link key for the given link parameters
	GetLinkKey(dialerBinding, protocol, dest, listenerBinding string) string

	// GetDestinationListeners returns a snapshot of the most recent
	// listener set advertised by each known destination router, keyed by
	// router id. Used by stale-link checks to compare a link's recorded
	// listener binding/groups against the destination's current state. The
	// bool is false when the snapshot could not be produced (e.g. the
	// registry event loop didn't respond in time); callers must not treat a
	// failed snapshot as "no destinations have listeners".
	GetDestinationListeners() (map[string][]*ctrl_pb.Listener, bool)

	// RescanForDialOpportunities re-evaluates every known link destination
	// against the *current* local dialer set, discovering matches that
	// became possible after a local dialer change (e.g., a new dialer
	// binding registered, a dialer's groups expanded). Without this,
	// changes to local dialers only take effect when peers' listener
	// state changes (via UpdateLinkDest), because that's the only path
	// that re-evaluates matches today.
	RescanForDialOpportunities()
}

type Forwarder interface {
	ForwardPayload(srcAddr xgress.Address, payload *xgress.Payload, timeout time.Duration) error
	ForwardAcknowledgement(srcAddr xgress.Address, acknowledgement *xgress.Acknowledgement) error
	ForwardControl(srcAddr xgress.Address, control *xgress.Control) error
}

// A Factory creates link listeners and link dialers
type Factory interface {
	CreateListener(id *identity.TokenId, config transport.Configuration) (Listener, error)
	CreateDialer(id *identity.TokenId, config transport.Configuration) (Dialer, error)
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
	GetLinkId() string
	GetRouterId() string
	GetAddress() string
	GetLinkProtocol() string
	GetRouterVersion() string
	GetIteration() uint32
	// GetListenerBinding returns the local binding of the remote
	// listener this dial targets. Recorded on the xlink at dial time so
	// stale-link checks can later look the listener up in the
	// destination snapshot.
	GetListenerBinding() string
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
	SendPayload(payload *xgress.Payload, timeout time.Duration, payloadType xgress.PayloadType) error
	SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error
	SendControl(control *xgress.Control) error
	InspectCircuit(circuitDetail *xgress.CircuitInspectDetail)
	GetDestinationType() string
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
	// LinkKey returns the structured identity of the link. The string
	// form (used as the registry map key) is available via Key(); the
	// struct form exposes individual components (dialerBinding,
	// protocol, destId, listenerBinding) for stale-link checks and
	// other code that needs to reason about specific parts of the key.
	LinkKey() LinkKey
	CloseOnce(f func())
	IsClosed() bool
	// SetHeartbeatControl registers a channel's heartbeat control handle with
	// the link so its intervals can be retuned later. A split link has one per
	// underlying channel.
	SetHeartbeatControl(hc channel.HeartbeatControl)
	// UpdateHeartbeatIntervals retunes the heartbeat send and check intervals
	// on every channel backing the link, applying a managed-config change to an
	// established link without rebuilding it.
	UpdateHeartbeatIntervals(sendInterval, checkInterval time.Duration)
	InspectLink() *inspect.LinkInspectDetail
	GetLinkConnState() *ctrl_pb.LinkConnState
	IsDialed() bool
	Iteration() uint32
	AreFaultsSent() bool
	DuplicatesRejected() uint32
}

type MultiConnXLink interface {
	Xlink
	MarkLinkStateSynced(ctrlId string)
	MarkLinkStateSyncedForState(ctrlId string, stateId string)
	GetCtrlRequiringSync() (string, []string)
}
