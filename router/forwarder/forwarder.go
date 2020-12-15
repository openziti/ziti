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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/util/info"
	"github.com/pkg/errors"
	"time"
)

type Forwarder struct {
	sessions        *sessionTable
	destinations    *destinationTable
	metricsRegistry metrics.UsageRegistry
	traceController trace.Controller
	Options         *Options
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

func NewForwarder(metricsRegistry metrics.UsageRegistry, options *Options) *Forwarder {
	forwarder := &Forwarder{
		sessions:        newSessionTable(),
		destinations:    newDestinationTable(),
		metricsRegistry: metricsRegistry,
		traceController: trace.NewController(),
		Options:         options,
	}

	return forwarder
}

func (forwarder *Forwarder) MetricsRegistry() metrics.UsageRegistry {
	return forwarder.metricsRegistry
}

func (forwarder *Forwarder) TraceController() trace.Controller {
	return forwarder.traceController
}

func (forwarder *Forwarder) RegisterDestination(sessionId string, address xgress.Address, destination Destination) {
	forwarder.destinations.addDestination(address, destination)
	forwarder.destinations.linkDestinationToSession(sessionId, address)
}

func (forwarder *Forwarder) UnregisterDestinations(sessionId string) {
	if addresses, found := forwarder.destinations.getAddressesForSession(sessionId); found {
		for _, address := range addresses {
			if destination, found := forwarder.destinations.getDestination(address); found {
				forwarder.destinations.removeDestination(address)
				go destination.(XgressDestination).Close() // create close queue?
			}
		}
		forwarder.destinations.unlinkSession(sessionId)
	}
}

func (forwarder *Forwarder) HasDestination(address xgress.Address) bool {
	_, found := forwarder.destinations.getDestination(address)
	return found
}

func (forwarder *Forwarder) RegisterLink(link xlink.Xlink) {
	forwarder.destinations.addDestination(xgress.Address(link.Id().Token), link)
}

func (forwarder *Forwarder) UnregisterLink(link xlink.Xlink) {
	forwarder.destinations.removeDestination(xgress.Address(link.Id().Token))
}

func (forwarder *Forwarder) Route(route *ctrl_pb.Route) {
	sessionId := route.SessionId
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

func (forwarder *Forwarder) Unroute(sessionId string, now bool) {
	if now {
		forwarder.sessions.removeForwardTable(sessionId)
		forwarder.EndSession(sessionId)
	} else {
		go forwarder.unrouteTimeout(sessionId, 5000)
	}
}

func (forwarder *Forwarder) EndSession(sessionId string) {
	forwarder.UnregisterDestinations(sessionId)
}

func (forwarder *Forwarder) ForwardPayload(srcAddr xgress.Address, payload *xgress.Payload) error {
	log := pfxlog.ContextLogger(string(srcAddr))

	sessionId := payload.GetSessionId()
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
				return errors.Errorf("cannot forward payload, no destination for session=%v src=%v dst=%v", sessionId, srcAddr, dstAddr)
			}
		} else {
			return errors.Errorf("cannot forward payload, no destination address for session=%v src=%v", sessionId, srcAddr)
		}
	} else {
		return errors.Errorf("cannot forward payload, no forward table for session=%v src=%v", sessionId, srcAddr)
	}
}

func (forwarder *Forwarder) ForwardAcknowledgement(srcAddr xgress.Address, acknowledgement *xgress.Acknowledgement) error {
	log := pfxlog.ContextLogger(string(srcAddr))

	sessionId := acknowledgement.SessionId
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
				return errors.Errorf("cannot acknowledge, no destination for session=%v src=%v dst=%v", sessionId, srcAddr, dstAddr)
			}

		} else {
			return errors.Errorf("cannot acknowledge, no destination address for session=%v src=%v", sessionId, srcAddr)
		}

	} else {
		return errors.Errorf("cannot acknowledge, no forward table for session=%v src=%v", sessionId, srcAddr)
	}
}

func (forwarder *Forwarder) Debug() string {
	return forwarder.sessions.debug() + forwarder.destinations.debug()
}

// unrouteTimeout implements a goroutine to manage route timeout processing. Once a timeout processor has been launched
// for a session, it will be checked repeatedly, looking to see if the session has crossed the inactivity threshold.
// Once it crosses the inactivity threshold, it gets removed.
//
func (forwarder *Forwarder) unrouteTimeout(sessionId string, ms int64) {
	log := pfxlog.ContextLogger("s/" + sessionId)
	log.Debug("scheduled")
	defer log.Debug("timeout")

	for {
		time.Sleep(time.Duration(ms) * time.Millisecond)

		if ft, found := forwarder.sessions.getForwardTable(sessionId); found {
			elapsedDelta := info.NowInMilliseconds() - ft.lastUsed
			if elapsedDelta >= ms {
				forwarder.sessions.removeForwardTable(sessionId)
				forwarder.EndSession(sessionId)
				return
			}

		} else {
			return
		}
	}
}
