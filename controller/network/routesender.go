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

package network

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"time"
)

type routeSenderController struct {
	senders cmap.ConcurrentMap // map[string]*routeSender
}

func newRouteSenderController() *routeSenderController {
	return &routeSenderController{senders: cmap.New()}
}

func (self *routeSenderController) forwardRouteResult(r *Router, circuitId string, attempt uint32, success bool, rerr string, peerData xt.PeerData) bool {
	v, found := self.senders.Get(circuitId)
	if found {
		routeSender := v.(*routeSender)
		routeSender.in <- &routeStatus{r: r, circuitId: circuitId, attempt: attempt, success: success, rerr: rerr, peerData: peerData}
		return true
	}
	logrus.Warnf("did not find route sender for [s/%s]", circuitId)
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
	path            *Path
	routeMsgs       []*ctrl_pb.Route
	timeout         time.Duration
	in              chan *routeStatus
	attendance      map[string]bool
	serviceCounters ServiceCounters
}

func newRouteSender(circuitId string, timeout time.Duration, serviceCounters ServiceCounters) *routeSender {
	return &routeSender{
		circuitId:       circuitId,
		timeout:         timeout,
		in:              make(chan *routeStatus, 16),
		attendance:      make(map[string]bool),
		serviceCounters: serviceCounters,
	}
}

func (self *routeSender) route(attempt uint32, path *Path, routeMsgs []*ctrl_pb.Route, strategy xt.Strategy, terminator xt.Terminator, ctx logcontext.Context) (peerData xt.PeerData, cleanups map[string]struct{}, err error) {
	logger := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(ctx)

	// send route messages
	tr := path.Nodes[len(path.Nodes)-1]
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
			if status.success {
				if status.attempt == attempt {
					logger.Debugf("received successful route status from [r/%s] for attempt [#%d] of [s/%s]", status.r.Id, status.attempt, status.circuitId)

					self.attendance[status.r.Id] = true
					if status.r == tr {
						peerData = status.peerData
						strategy.NotifyEvent(xt.NewDialSucceeded(terminator))
						self.serviceCounters.ServiceDialSuccess(terminator.GetServiceId())
					}
				} else {
					logger.Warnf("received successful route status from [r/%s] for alien attempt [#%d (not #%d)] of [s/%s]", status.r.Id, status.attempt, attempt, status.circuitId)
				}

			} else {
				if status.attempt == attempt {
					logger.Warnf("received failed route status from [r/%s] for attempt [#%d] of [s/%s] (%v)", status.r.Id, status.attempt, status.circuitId, status.rerr)

					if status.r == tr {
						strategy.NotifyEvent(xt.NewDialFailedEvent(terminator))
						self.serviceCounters.ServiceDialFail(terminator.GetServiceId())
					}
					cleanups = self.cleanups(path)

					return nil, cleanups, errors.Errorf("error creating route for [s/%s] on [r/%s] (%v)", self.circuitId, status.r.Id, status.rerr)
				} else {
					logger.Warnf("received failed route status from [r/%s] for alien attempt [#%d (not #%d)] of [s/%s]", status.r.Id, status.attempt, attempt, status.circuitId)
				}
			}

		case <-time.After(timeout):
			cleanups = self.cleanups(path)
			self.serviceCounters.ServiceDialTimeout(terminator.GetServiceId())
			return nil, cleanups, &routeTimeoutError{circuitId: self.circuitId}
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

func (self *routeSender) sendRoute(r *Router, routeMsg *ctrl_pb.Route, ctx logcontext.Context) {
	logger := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(ctx)

	body, err := proto.Marshal(routeMsg)
	if err != nil {
		logger.Errorf("error marshalling route message to [r/%s] (%v)", r.Id, err)
		return
	}
	if err := r.Control.Send(channel2.NewMessage(int32(ctrl_pb.ContentType_RouteType), body)); err != nil {
		logger.WithError(err).Errorf("failure sending route message to [r/%s]", r.Id)
	} else {
		logger.Debugf("sent route message to [r/%s]", r.Id)
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

type routeStatus struct {
	r         *Router
	circuitId string
	attempt   uint32
	success   bool
	rerr      string
	peerData  xt.PeerData
}

type routeTimeoutError struct {
	circuitId string
}

func (self routeTimeoutError) Error() string {
	return fmt.Sprintf("timeout creating routes for [s/%s]", self.circuitId)
}
