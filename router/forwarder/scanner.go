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

package forwarder

import (
	"github.com/openziti/channel"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/sirupsen/logrus"
	"sync/atomic"
	"time"
)

type Scanner struct {
	ctrl        channel.Channel
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

func (self *Scanner) SetCtrl(ch channel.Channel) {
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
	now := time.Now().UnixMilli()
	for circuitId, ft := range circuits {
		idleTime := time.Duration(now-atomic.LoadInt64(&ft.last)) * time.Millisecond
		if idleTime > self.timeout {
			idleCircuitIds = append(idleCircuitIds, circuitId)
			logrus.WithField("circuitId", circuitId).
				WithField("idleTime", idleTime).
				WithField("idleThreshold", self.timeout).
				Warn("circuit exceeds idle threshold")
		}
	}

	if len(idleCircuitIds) > 0 {
		logrus.Debugf("found [%d] idle circuits, confirming with controller", len(idleCircuitIds))

		if self.ctrl != nil {
			confirm := &ctrl_pb.CircuitConfirmation{CircuitIds: idleCircuitIds}
			if err := protobufs.MarshalTyped(confirm).Send(self.ctrl); err == nil {
				logrus.WithField("circuitCount", len(idleCircuitIds)).Warnf("sent confirmation for circuits")
			} else {
				logrus.WithError(err).Error("error sending confirmation request")
			}
		} else {
			logrus.Errorf("no ctrl channel, cannot request circuit confirmations")
		}
	}
}
