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
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/sirupsen/logrus"
	"time"
)

type Scanner struct {
	ctrl        channel2.Channel
	circuits    *circuitTable
	interval    time.Duration
	timeout     time.Duration
	closeNotify <-chan struct{}
}

func NewScanner(options *Options, closeNotify <-chan struct{}) *Scanner {
	s := &Scanner{
		interval:    options.IdleTxInterval,
		timeout:     options.IdleCircuitTimeout,
		closeNotify: closeNotify,
	}
	if s.interval > 0 {
		go s.run()
	} else {
		logrus.Warnf("scanner disabled")
	}
	return s
}

func (self *Scanner) SetCtrl(ch channel2.Channel) {
	self.ctrl = ch
}

func (self *Scanner) setCircuitTable(circuits *circuitTable) {
	self.circuits = circuits
}

func (self *Scanner) run() {
	logrus.Info("started")
	defer logrus.Warn("exited")

	for {
		select {
		case <-time.After(self.interval):
			self.scan()

		case <-self.closeNotify:
			return
		}
	}
}

func (self *Scanner) scan() {
	circuits := self.circuits.circuits.Items()
	logrus.Debugf("scanning [%d] circuits", len(circuits))

	var idleCircuitIds []string
	for circuitId, ft := range circuits {
		if time.Since(ft.(*forwardTable).last) > self.timeout {
			idleCircuitIds = append(idleCircuitIds, circuitId)
			logrus.Warnf("[s/%s] idle after [%s]", circuitId, self.timeout)
		}
	}

	if len(idleCircuitIds) > 0 {
		logrus.Debugf("found [%d] idle circuits, confirming with controller", len(idleCircuitIds))

		if self.ctrl != nil {
			confirm := &ctrl_pb.CircuitConfirmation{CircuitIds: idleCircuitIds}
			body, err := proto.Marshal(confirm)
			if err == nil {
				msg := channel2.NewMessage(ctrl_msg.CircuitConfirmationType, body)
				if err := self.ctrl.Send(msg); err == nil {
					logrus.Warnf("sent confirmation for [%d] circuits", len(idleCircuitIds))
				} else {
					logrus.Errorf("error sending confirmation request (%v)", err)
				}
			}
		} else {
			logrus.Errorf("no ctrl channel, cannot request circuit confirmations")
		}
	}
}
