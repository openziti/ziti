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
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/env"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
)

type Faulter struct {
	ctrls       env.NetworkControllers
	interval    time.Duration
	circuitIds  cmap.ConcurrentMap[string]
	closeNotify chan struct{}
}

func NewFaulter(ctrls env.NetworkControllers, interval time.Duration, closeNotify chan struct{}) *Faulter {
	f := &Faulter{
		ctrls:       ctrls,
		interval:    interval,
		circuitIds:  cmap.New[string](),
		closeNotify: closeNotify,
	}

	if interval > 0 {
		go f.run()
	}

	return f
}

func (self *Faulter) report(circuitId string, ctrlId string) {
	if self.interval > 0 {
		self.circuitIds.Set(circuitId, ctrlId)
	}
}

func (self *Faulter) notifyInvalidLink(linkId string) {
	log := pfxlog.Logger()
	self.ctrls.ForEach(func(ctrlId string, ch channel.Channel) {
		fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_LinkFault, Id: linkId}
		if err := protobufs.MarshalTyped(fault).WithTimeout(self.ctrls.DefaultRequestTimeout()).Send(ch); err != nil {
			log.WithError(err).
				WithField("ctrlId", ctrlId).
				WithField("linkId", linkId).
				Error("failed to notify of invalid link")
		}
	})
}

func (self *Faulter) run() {
	logrus.Infof("started")
	defer logrus.Errorf("exited")

	for {
		select {
		case <-time.After(self.interval):
			workloadByCtrl := map[string][]string{}
			self.circuitIds.IterCb(func(circuitId, ctrlId string) {
				workloadByCtrl[ctrlId] = append(workloadByCtrl[ctrlId], circuitId)
			})

			for ctrlId, workload := range workloadByCtrl {
				// Proactively remove from reported circuitIds. If we fail below, forwarder will continue to report.
				for _, circuitId := range workload {
					self.circuitIds.Remove(circuitId)
				}

				circuitIds := strings.Join(workload, " ")
				fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_ForwardFault, Id: circuitIds}

				log := pfxlog.Logger().WithField("ctrlId", ctrlId)
				ch := self.ctrls.GetCtrlChannel(ctrlId)
				if ch == nil {
					log.Error("no control channel for controller")
					continue
				}

				if err := protobufs.MarshalTyped(fault).Send(ch); err == nil {
					log.WithField("circuitCount", len(workload)).Warn("reported forwarding faults")
				} else {
					log.WithError(err).Error("error sending fault report")
				}
			}

		case <-self.closeNotify:
			return
		}
	}
}
