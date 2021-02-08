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
	"github.com/golang/protobuf/proto"
	"github.com/openziti/fabric/controller/xt"
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

func (self *routeSenderController) forwardRouteResult(r *Router, sessionId string, success bool, peerData xt.PeerData) bool {
	v, found := self.senders.Get(sessionId)
	if found {
		logrus.Infof("found route sender for [s/%s]", sessionId)
		routeSender := v.(*routeSender)
		routeSender.in <- &routeStatus{r: r, sessionId: sessionId, success: success, peerData: peerData}
		return true
	}
	logrus.Warnf("did not find route sender for [s/%s]", sessionId)
	return false
}

func (self *routeSenderController) addRouteSender(rs *routeSender) {
	self.senders.Set(rs.sessionId, rs)
}

func (self *routeSenderController) removeRouteSender(rs *routeSender) {
	self.senders.Remove(rs.sessionId)
}

type routeSender struct {
	sessionId  string
	circuit    *Circuit
	routeMsgs  []*ctrl_pb.Route
	timeout    time.Duration
	in         chan *routeStatus
	attendance map[string]bool
}

func newRouteSender(sessionId string, timeout time.Duration) *routeSender {
	return &routeSender{
		sessionId:  sessionId,
		timeout:    timeout,
		in:         make(chan *routeStatus, 16),
		attendance: make(map[string]bool),
	}
}

func (self *routeSender) route(circuit *Circuit, routeMsgs []*ctrl_pb.Route, strategy xt.Strategy, terminator xt.Terminator) (xt.PeerData, error) {
	// send route messages
	tr := circuit.Path[len(circuit.Path)-1]
	for i := 0; i < len(circuit.Path); i++ {
		r := circuit.Path[i]
		go self.sendRoute(r, routeMsgs[i])
		self.attendance[r.Id] = false
	}

	var peerData xt.PeerData
	deadline := time.Now().Add(self.timeout)
	timeout := time.Until(deadline)
attendance:
	for {
		select {
		case status := <-self.in:
			logrus.Infof("received status from [r/%s] for [s/%s]: %v", status.r.Id, status.sessionId, status)

			if status.success {
				self.attendance[status.r.Id] = true
				if status.r == tr {
					peerData = status.peerData
					strategy.NotifyEvent(xt.NewDialSucceeded(terminator))
				}

			} else {
				if status.r == tr {
					strategy.NotifyEvent(xt.NewDialFailedEvent(terminator))
				}
				self.revert(self.sessionId, circuit)
				return nil, errors.Errorf("error creating route for [s/%s] on [r/%s]", self.sessionId, status.r.Id)
			}

		case <-time.After(timeout):
			self.revert(self.sessionId, circuit)
			return nil, errors.Errorf("timeout creating routes for [s/%s]", self.sessionId)
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

	return peerData, nil
}

func (self *routeSender) sendRoute(r *Router, routeMsg *ctrl_pb.Route) {
	body, err := proto.Marshal(routeMsg)
	if err != nil {
		logrus.Errorf("error marshalling route message for [s/%s] to [r/%s] (%v)", routeMsg.SessionId, r.Id, err)
		return
	}
	r.Control.Send(channel2.NewMessage(int32(ctrl_pb.ContentType_RouteType), body))

	logrus.Infof("sent route message for [s/%s] to [r/%s]", routeMsg.SessionId, r.Id)
}

func (self *routeSender) revert(sessionId string, circuit *Circuit) {
	for _, r := range circuit.Path {
		success, found := self.attendance[r.Id]
		if found && success {
			unrouteMsg := &ctrl_pb.Unroute{SessionId: sessionId, Now: true}
			if body, err := proto.Marshal(unrouteMsg); err == nil {
				r.Control.Send(channel2.NewMessage(int32(ctrl_pb.ContentType_UnrouteType), body))
				logrus.Warnf("sent cleanup unroute to [r/%s] for [s/%s]", r.Id, sessionId)
			} else {
				logrus.Errorf("error marshaling cleanup unroute to [r/%s] for [s/%s] (%v)", r.Id, sessionId, err)
			}
		}
	}
}

type routeStatus struct {
	r         *Router
	sessionId string
	success   bool
	peerData  xt.PeerData
}
