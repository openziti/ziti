/*
	Copyright NetFoundry, Inc.

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

package forwarder

import (
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-fabric/trace"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/metrics"
	"github.com/netfoundry/ziti-foundation/util/info"
	"time"
)

type Forwarder struct {
	sessions                *sessionTable
	destinations            *destinationTable
	payloadBufferController *xgress.PayloadBufferController
	metricsRegistry         metrics.Registry
	traceController         trace.Controller
}

type Destination interface {
	SendPayload(payload *xgress.Payload) error
	SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error
}

type XgressDestination interface {
	Destination
	Close()
	Start()
	IsTerminator() bool
	Label() string
}

func NewForwarder(metricsRegistry metrics.Registry) *Forwarder {
	forwarder := &Forwarder{
		sessions:        newSessionTable(),
		destinations:    newDestinationTable(),
		metricsRegistry: metricsRegistry,
		traceController: trace.NewController(),
	}

	forwarder.payloadBufferController = xgress.NewPayloadBufferController(forwarder)
	return forwarder
}

func (forwarder *Forwarder) PayloadBuffer(sessionId *identity.TokenId, address xgress.Address) *xgress.PayloadBuffer {
	return forwarder.payloadBufferController.BufferForSession(sessionId, address)
}

func (forwarder *Forwarder) PayloadBufferController() *xgress.PayloadBufferController {
	return forwarder.payloadBufferController
}

func (forwarder *Forwarder) MetricsRegistry() metrics.Registry {
	return forwarder.metricsRegistry
}

func (forwarder *Forwarder) TraceController() trace.Controller {
	return forwarder.traceController
}

func (forwarder *Forwarder) RegisterDestination(sessionId *identity.TokenId, address xgress.Address, destination Destination) {
	forwarder.destinations.addDestination(address, destination)
	forwarder.destinations.linkDestinationToSession(sessionId, address)
}

func (forwarder *Forwarder) UnregisterDestinations(sessionId *identity.TokenId) {
	if addresses, found := forwarder.destinations.getAddressesForSession(sessionId); found {
		for _, address := range addresses {
			if destination, found := forwarder.destinations.getDestination(address); found {
				forwarder.destinations.removeDestination(address)
				destination.(XgressDestination).Close()
			}
		}
		forwarder.destinations.unlinkSession(sessionId)
	}
}

func (forwarder *Forwarder) StartDestinations(sessionId *identity.TokenId) {
	if addresses, found := forwarder.destinations.getAddressesForSession(sessionId); found {
		for _, address := range addresses {
			if destination, found := forwarder.destinations.getDestination(address); found {
				if xgressDest, ok := destination.(XgressDestination); ok && xgressDest.IsTerminator() {
					xgressDest.Start()
					pfxlog.Logger().Infof("Started xgress for session %v", sessionId.Token)
				}
			}
		}
	}
}

func (forwarder *Forwarder) HasDestination(address xgress.Address) bool {
	_, found := forwarder.destinations.getDestination(address)
	return found
}

func (forwarder *Forwarder) RegisterLink(link *Link) {
	forwarder.destinations.addDestination(xgress.Address(link.Id.Token), link)
}

func (forwarder *Forwarder) UnregisterLink(link *Link) {
	forwarder.destinations.removeDestination(xgress.Address(link.Id.Token))
}

func (forwarder *Forwarder) Route(route *ctrl_pb.Route) {
	sessionId := &identity.TokenId{Token: route.SessionId}
	var sessionFt *forwardTable
	if ft, found := forwarder.sessions.getForwardTable(sessionId); found {
		sessionFt = ft
	} else {
		sessionFt = newForwardTable()
	}
	for _, forward := range route.Forwards {
		sessionFt.setForwardAddress(xgress.Address(forward.SrcAddress), xgress.Address(forward.DstAddress))
	}
	forwarder.sessions.setForwardTable(sessionId, sessionFt)
}

func (forwarder *Forwarder) Unroute(sessionId *identity.TokenId, now bool) {
	if now {
		forwarder.sessions.removeForwardTable(sessionId)
		_ = forwarder.EndSession(sessionId)
	} else {
		go forwarder.unrouteTimeout(sessionId, 5000)
	}
}

func (forwarder *Forwarder) EndSession(sessionId *identity.TokenId) error {
	forwarder.UnregisterDestinations(sessionId)
	forwarder.payloadBufferController.EndSession(sessionId)
	return nil
}

func (forwarder *Forwarder) ForwardPayload(srcAddr xgress.Address, payload *xgress.Payload) error {
	log := pfxlog.ContextLogger(string(srcAddr))

	sessionId := &identity.TokenId{Token: payload.GetSessionId()}
	if forwardTable, found := forwarder.sessions.getForwardTable(sessionId); found {
		if dstAddr, found := forwardTable.getForwardAddress(srcAddr); found {
			if dst, found := forwarder.destinations.getDestination(dstAddr); found {
				if err := dst.SendPayload(payload); err != nil {
					return err
				}
				log.WithFields(payload.GetLoggerFields()).Debugf("=> %s", string(dstAddr))
				forwardTable.lastUsed = info.NowInMilliseconds()
				return nil

			} else {
				return errors.New("no destination")
			}

		} else {
			return errors.New("no destination address")
		}

	} else {
		return errors.New("no forward table")
	}
}

func (forwarder *Forwarder) ForwardAcknowledgement(srcAddr xgress.Address, acknowledgement *xgress.Acknowledgement) error {
	log := pfxlog.ContextLogger(string(srcAddr))

	sessionId := &identity.TokenId{Token: acknowledgement.SessionId}
	if forwardTable, found := forwarder.sessions.getForwardTable(sessionId); found {
		if dstAddr, found := forwardTable.getForwardAddress(srcAddr); found {
			if dst, found := forwarder.destinations.getDestination(dstAddr); found {
				if err := dst.SendAcknowledgement(acknowledgement); err != nil {
					return err
				}
				log.Debugf("=> %s", string(dstAddr))
				forwardTable.lastUsed = info.NowInMilliseconds()
				return nil

			} else {
				return errors.New("no destination")
			}

		} else {
			return errors.New("no destination address")
		}

	} else {
		return errors.New("no forward table")
	}
}

func (forwarder *Forwarder) Debug() string {
	return forwarder.sessions.debug() + forwarder.destinations.debug()
}

// unrouteTimeout implements a goroutine to manage route timeout processing. Once a timeout processor has been launched
// for a session, it will be checked repeatedly, looking to see if the session has crossed the inactivity threshold.
// Once it crosses the inactivity threshold, it gets removed.
//
func (forwarder *Forwarder) unrouteTimeout(sessionId *identity.TokenId, ms int64) {
	log := pfxlog.ContextLogger("s/" + sessionId.Token)
	log.Info("scheduled")
	defer log.Warn("timeout")

	for {
		time.Sleep(time.Duration(ms) * time.Millisecond)

		if ft, found := forwarder.sessions.getForwardTable(sessionId); found {
			elapsedDelta := info.NowInMilliseconds() - ft.lastUsed
			if elapsedDelta >= ms {
				forwarder.sessions.removeForwardTable(sessionId)
				_ = forwarder.EndSession(sessionId)
				return
			}

		} else {
			return
		}
	}
}
