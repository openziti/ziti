/*
	Copyright NetFoundry Inc.

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

package network

import (
	"fmt"
	"github.com/openziti/fabric/controller/change"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/pb/ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/sirupsen/logrus"
)

type routeSenderController struct {
	senders cmap.ConcurrentMap[string, *routeSender]
}

func newRouteSenderController() *routeSenderController {
	return &routeSenderController{senders: cmap.New[*routeSender]()}
}

func (self *routeSenderController) forwardRouteResult(rs *RouteStatus) bool {
	sender, found := self.senders.Get(rs.CircuitId)
	if found {
		sender.in <- rs
		return true
	}
	logrus.Warnf("did not find route sender for [s/%s]", rs.CircuitId)
	return false
}

func (self *routeSenderController) addRouteSender(rs *routeSender) {
	self.senders.Set(rs.circuitId, rs)
}

func (self *routeSenderController) removeRouteSender(rs *routeSender) {
	self.senders.Remove(rs.circuitId)
}

type routeSender struct {
	circuitId       string
	timeout         time.Duration
	in              chan *RouteStatus
	attendance      map[string]bool
	serviceCounters ServiceCounters
	terminators     *TerminatorManager
}

func newRouteSender(circuitId string, timeout time.Duration, serviceCounters ServiceCounters, terminators *TerminatorManager) *routeSender {
	return &routeSender{
		circuitId:       circuitId,
		timeout:         timeout,
		in:              make(chan *RouteStatus, 16),
		attendance:      make(map[string]bool),
		serviceCounters: serviceCounters,
		terminators:     terminators,
	}
}

func (self *routeSender) route(attempt uint32, path *Path, routeMsgs []*ctrl_pb.Route, strategy xt.Strategy, terminator xt.Terminator, ctx logcontext.Context) (peerData xt.PeerData, cleanups map[string]struct{}, err CircuitError) {
	logger := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(ctx)

	// send route messages
	for i := 0; i < len(path.Nodes); i++ {
		r := path.Nodes[i]
		msg := routeMsgs[i]
		logger.Debugf("sending route message to [r/%s] for attempt [#%d]", r.Id, msg.Attempt)
		go self.sendRoute(r, msg, ctx)
		self.attendance[r.Id] = false
	}

	deadline := time.Now().Add(self.timeout)
	timeout := time.Until(deadline)
attendance:
	for {
		select {
		case status := <-self.in:
			var tmpPeerData xt.PeerData
			tmpPeerData, cleanups, err = self.handleRouteSend(attempt, path, strategy, status, terminator, logger)
			if err != nil {
				return nil, cleanups, err
			}
			if tmpPeerData != nil && status.Attempt == attempt && status.Router.Id == terminator.GetRouterId() {
				peerData = tmpPeerData
			}

		case <-time.After(timeout):
			cleanups = self.cleanups(path)
			strategy.NotifyEvent(xt.NewDialFailedEvent(terminator))
			self.serviceCounters.ServiceDialTimeout(terminator.GetServiceId(), terminator.GetId())
			return nil, cleanups, newCircuitErrWrap(CircuitFailureRouterErrDialConnRefused, &routeTimeoutError{circuitId: self.circuitId})
		}

		allPresent := true
		for _, v := range self.attendance {
			if !v {
				allPresent = false
			}
		}
		if allPresent {
			break attendance
		}

		timeout = time.Until(deadline)
	}

	return peerData, nil, nil
}

func (self *routeSender) handleRouteSend(attempt uint32, path *Path, strategy xt.Strategy, status *RouteStatus, terminator xt.Terminator, logger *pfxlog.Builder) (peerData xt.PeerData, cleanups map[string]struct{}, err CircuitError) {
	if status.Success == (status.ErrorCode != nil) {
		logger.Errorf("route status success and error code differ. Success: %v ErrorCode: %v", status.Success, status.ErrorCode)
	}

	if status.Success {
		if status.Attempt == attempt {
			logger.Debugf("received successful route status from [r/%s] for attempt [#%d] of [s/%s]", status.Router.Id, status.Attempt, status.CircuitId)

			self.attendance[status.Router.Id] = true
			if status.Router.Id == terminator.GetRouterId() {
				peerData = status.PeerData
				strategy.NotifyEvent(xt.NewDialSucceeded(terminator))
				self.serviceCounters.ServiceDialSuccess(terminator.GetServiceId(), terminator.GetId())
			}
		} else {
			logger.Warnf("received successful route status from [r/%s] for alien attempt [#%d (not #%d)] of [s/%s]", status.Router.Id, status.Attempt, attempt, status.CircuitId)
		}
		return peerData, nil, nil
	}

	if status.Attempt == attempt {
		failureCause := CircuitFailureRouterErrGeneric

		var errorCode byte
		if status.ErrorCode != nil {
			errorCode = *status.ErrorCode
		}

		switch errorCode {
		case ctrl_msg.ErrorTypeGeneric:
			self.serviceCounters.ServiceDialOtherError(terminator.GetServiceId())
		case ctrl_msg.ErrorTypeInvalidTerminator:
			if terminator.GetBinding() == "edge" || terminator.GetBinding() == "tunnel" {
				self.serviceCounters.ServiceInvalidTerminator(terminator.GetServiceId(), terminator.GetId())
				changeCtx := change.New().
					SetChangeAuthorId(status.Router.Id).
					SetChangeAuthorName(status.Router.Name).
					SetSource("router reported invalid terminator")
				if err := self.terminators.Delete(terminator.GetId(), changeCtx); err != nil {
					logger.WithError(fmt.Errorf("unable to delete invalid terminator: %v", err))
				}
				failureCause = CircuitFailureRouterErrInvalidTerminator
			} else {
				self.serviceCounters.ServiceMisconfiguredTerminator(terminator.GetServiceId(), terminator.GetId())
				self.terminators.handlePrecedenceChange(terminator.GetId(), xt.Precedences.Failed)
				failureCause = CircuitFailureRouterErrMisconfiguredTerminator
			}
		case ctrl_msg.ErrorTypeDialTimedOut:
			self.serviceCounters.ServiceTerminatorTimeout(terminator.GetServiceId(), terminator.GetId())
			failureCause = CircuitFailureRouterErrDialTimedOut
		case ctrl_msg.ErrorTypeConnectionRefused:
			self.serviceCounters.ServiceTerminatorConnectionRefused(terminator.GetServiceId(), terminator.GetId())
			failureCause = CircuitFailureRouterErrDialConnRefused
		default:
			logger.WithField("errorCode", status.ErrorCode).Error("unhandled error code")
		}

		logger.Warnf("received failed route status from [r/%s] for attempt [#%d] of [s/%s] (%v)", status.Router.Id, status.Attempt, status.CircuitId, status.Err)

		if status.Router.Id == terminator.GetRouterId() {
			strategy.NotifyEvent(xt.NewDialFailedEvent(terminator))
			self.serviceCounters.ServiceDialFail(terminator.GetServiceId(), terminator.GetId())
		}
		cleanups = self.cleanups(path)

		return nil, cleanups, newCircuitErrorf(failureCause, "error creating route for [s/%s] on [r/%s] (%v)", self.circuitId, status.Router.Id, status.Err)
	}

	logger.Warnf("received failed route status from [r/%s] for alien attempt [#%d (not #%d)] of [s/%s]", status.Router.Id, status.Attempt, attempt, status.CircuitId)
	return nil, nil, nil
}

func (self *routeSender) sendRoute(r *Router, routeMsg *ctrl_pb.Route, ctx logcontext.Context) {
	logger := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(ctx).WithField("routerId", r.Id)

	envelope := protobufs.MarshalTyped(routeMsg).WithTimeout(3 * time.Second)
	if err := envelope.SendAndWaitForWire(r.Control); err != nil {
		logger.WithError(err).Error("failure sending route message")
	} else {
		logger.Debug("sent route message")
	}
}

func (self *routeSender) cleanups(path *Path) map[string]struct{} {
	cleanups := make(map[string]struct{})
	for _, r := range path.Nodes {
		success, found := self.attendance[r.Id]
		if found && success {
			cleanups[r.Id] = struct{}{}
		}
	}
	return cleanups
}

type RouteStatus struct {
	Router    *Router
	CircuitId string
	Attempt   uint32
	Success   bool
	Err       string
	PeerData  xt.PeerData
	ErrorCode *byte
}

type routeTimeoutError struct {
	circuitId string
}

func (self routeTimeoutError) Error() string {
	return fmt.Sprintf("timeout creating routes for [s/%s]", self.circuitId)
}
