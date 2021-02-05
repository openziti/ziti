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
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
	"time"
)

type routeSenderController struct {
	senders cmap.ConcurrentMap // map[string]*routeSender
}

func newRouteSenderController() *routeSenderController {
	return &routeSenderController{}
}

func (self *routeSenderController) forwardRouteResult(r *Router, sessionId string, success bool) bool {
	v, found := self.senders.Get(sessionId)
	if found {
		routeSender := v.(*routeSender)
		routeSender.in <- &routeStatus{r: r, sessionId: sessionId, success: success}
		return true
	}
	return false
}

type routeSender struct {
	sessionId  string
	circuit    *Circuit
	routeMsgs  []*ctrl_pb.Route
	timeout    time.Duration
	maxTries   int
	in         chan *routeStatus
	attendance map[string]bool
}

func newRouteSender(sessionId string, timeout time.Duration, maxTries int) *routeSender {
	return &routeSender{
		sessionId:  sessionId,
		timeout:    timeout,
		maxTries:   maxTries,
		attendance: make(map[string]bool),
	}
}

func (self *routeSender) route(circuit *Circuit, routeMsgs []*ctrl_pb.Route) error {
	defer func() {
		// remove sender
	}()

	// send route messages
	for i := 0; i < len(circuit.Path); i++ {
		r := circuit.Path[i]
		if err := self.sendRoute(r, routeMsgs[i]); err != nil {
			return errors.Wrapf(err, "unable to send route to [r/%s]", r.Id)
		}
		self.attendance[r.Id] = false
	}

	deadline := time.Now().Add(self.timeout)
	timeout := time.Until(deadline)
	for {
		select {
		case status := <-self.in:
			if status.success {
				self.attendance[status.r.Id] = true
				timeout = time.Until(deadline)

			} else {
				if status.r == circuit.Path[len(circuit.Path)-1] {
					// if this was a terminator, do retry logic

				} else {
					self.tearDownTheSuccesful()
					return errors.Errorf("error creating route for [s/%s] on [r/%s]", self.sessionId, status.r.Id)
				}
			}

		case <-time.After(timeout):
			self.tearDownTheSuccesful()
			return errors.Errorf("timeout creating routes for [s/%s]", self.sessionId)
		}
	}
}

func (self *routeSender) sendRoute(r *Router, routeMsg *ctrl_pb.Route) error {
	body, err := proto.Marshal(routeMsg)
	if err != nil {
		return errors.Wrap(err, "marshal protobuf")
	}
	r.Control.Send(channel2.NewMessage(int32(ctrl_pb.ContentType_RouteType), body))
	return nil
}

func (self *routeSender) tearDownTheSuccesful() {
}

type routeStatus struct {
	r         *Router
	sessionId string
	success   bool
}
