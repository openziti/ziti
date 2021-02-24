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
	"github.com/golang/protobuf/proto"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
)

type Faulter struct {
	ctrl        channel2.Channel
	interval    time.Duration
	sessionIds  cmap.ConcurrentMap // map[sessionId]struct{}
	closeNotify chan struct{}
}

func NewFaulter(interval time.Duration, closeNotify chan struct{}) *Faulter {
	f := &Faulter{interval: interval, sessionIds: cmap.New(), closeNotify: closeNotify}
	if interval > 0 {
		go f.run()
	}
	return f
}

func (self *Faulter) SetCtrl(ch channel2.Channel) {
	self.ctrl = ch
}

func (self *Faulter) report(sessionId string) {
	if self.interval > 0 {
		self.sessionIds.Set(sessionId, struct{}{})
	}
}

func (self *Faulter) run() {
	logrus.Infof("started")
	defer logrus.Errorf("exited")

	for {
		select {
		case <-time.After(self.interval):
			workload := self.sessionIds.Keys()
			if len(workload) > 0 {
				// Proactively remove from reported sessionIds. If we fail below, forwarder will continue to report.
				for _, sessionId := range workload {
					self.sessionIds.Remove(sessionId)
				}

				sessionIds := strings.Join(workload, " ")
				fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_ForwardFault, Id: sessionIds}
				body, err := proto.Marshal(fault)
				if err == nil {
					msg := channel2.NewMessage(int32(ctrl_pb.ContentType_FaultType), body)
					if err := self.ctrl.Send(msg); err == nil {
						logrus.Warnf("reported [%d] forwarding faults", len(workload))
					} else {
						logrus.Errorf("error sending fault report (%v)", err)
					}
				}
			}

		case <-self.closeNotify:
			return
		}
	}
}
