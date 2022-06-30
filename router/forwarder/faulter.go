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
	"github.com/openziti/channel"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
)

type Faulter struct {
	ctrl        channel.Channel
	interval    time.Duration
	circuitIds  cmap.ConcurrentMap[struct{}]
	closeNotify chan struct{}
}

func NewFaulter(interval time.Duration, closeNotify chan struct{}) *Faulter {
	f := &Faulter{interval: interval, circuitIds: cmap.New[struct{}](), closeNotify: closeNotify}
	if interval > 0 {
		go f.run()
	}
	return f
}

func (self *Faulter) SetCtrl(ch channel.Channel) {
	self.ctrl = ch
}

func (self *Faulter) report(circuitId string) {
	if self.interval > 0 {
		self.circuitIds.Set(circuitId, struct{}{})
	}
}

func (self *Faulter) notifyInvalidLink(linkId string) {
	log := pfxlog.Logger()
	fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_LinkFault, Id: linkId}
	if err := protobufs.MarshalTyped(fault).Send(self.ctrl); err != nil {
		log.WithError(err).WithField("linkId", linkId).Error("failed to notify of invalid link")
	}
}

func (self *Faulter) run() {
	logrus.Infof("started")
	defer logrus.Errorf("exited")

	for {
		select {
		case <-time.After(self.interval):
			workload := self.circuitIds.Keys()
			if len(workload) > 0 {
				// Proactively remove from reported circuitIds. If we fail below, forwarder will continue to report.
				for _, circuitId := range workload {
					self.circuitIds.Remove(circuitId)
				}

				circuitIds := strings.Join(workload, " ")
				fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_ForwardFault, Id: circuitIds}
				if err := protobufs.MarshalTyped(fault).Send(self.ctrl); err == nil {
					logrus.WithField("circuitCount", len(workload)).Warn("reported forwarding faults")
				} else {
					logrus.WithError(err).Error("error sending fault report")
				}
			}

		case <-self.closeNotify:
			return
		}
	}
}
