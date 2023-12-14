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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/sirupsen/logrus"
	"sync/atomic"
	"time"
)

type Scanner struct {
	ctrls       env.NetworkControllers
	circuits    *circuitTable
	interval    time.Duration
	timeout     time.Duration
	closeNotify <-chan struct{}
}

func newScanner(ctrls env.NetworkControllers, options *Options, closeNotify <-chan struct{}) *Scanner {
	s := &Scanner{
		ctrls:       ctrls,
		interval:    options.IdleTxInterval,
		timeout:     options.IdleCircuitTimeout,
		closeNotify: closeNotify,
	}
	return s
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

	now := time.Now().UnixMilli()
	idleCircuits := map[string]map[string]int64{}
	for circuitId, ft := range circuits {
		idleTime := time.Duration(now-atomic.LoadInt64(&ft.last)) * time.Millisecond
		if idleTime > self.timeout {
			ctrlMap := idleCircuits[ft.ctrlId]
			if ctrlMap == nil {
				ctrlMap = map[string]int64{}
				idleCircuits[ft.ctrlId] = ctrlMap
			}
			ctrlMap[circuitId] = int64(idleTime)
			logrus.WithField("circuitId", circuitId).
				WithField("ctrlId", ft.ctrlId).
				WithField("idleTime", idleTime).
				WithField("idleThreshold", self.timeout).
				Warn("circuit exceeds idle threshold")
		}
	}

	for ctrlId, idleCircuitMap := range idleCircuits {
		if len(idleCircuitMap) > 0 {
			log := pfxlog.Logger().WithField("ctrlId", ctrlId)
			log.Debugf("found [%d] idle circuits, confirming with controller", len(idleCircuitMap))

			if ctrl := self.ctrls.GetCtrlChannel(ctrlId); ctrl != nil {
				confirm := &ctrl_pb.CircuitConfirmation{IdleTimes: idleCircuitMap}
				for circuitId := range idleCircuitMap {
					confirm.CircuitIds = append(confirm.CircuitIds, circuitId)
				}
				if err := protobufs.MarshalTyped(confirm).Send(ctrl); err == nil {
					log.WithField("circuitCount", len(idleCircuitMap)).Warnf("sent confirmation for circuits")
				} else {
					log.WithError(err).Error("error sending confirmation request")
				}
			} else {
				log.Errorf("no ctrl channel, cannot request circuit confirmations")
			}
		}
	}
}
