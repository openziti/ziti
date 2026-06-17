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
	"strings"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/logging"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	cmap "github.com/orcaman/concurrent-map/v2"
)

// faultLog is the logger for circuit and link fault reporting. Its channel
// name is "router.forwarder.fault"; operators can adjust fault visibility
// with `ziti agent set-channel-log-level router.forwarder.fault <level>`.
//
// Fault reporting is the diagnostic surface between the router's
// data-plane and the controllers; it lives in its own channel so a noisy
// fault loop can be silenced without losing forward or scan output.
var faultLog = logging.For("router.forwarder.fault")

type Faulter struct {
	ctrls         env.NetworkControllers
	interval      time.Duration
	circuitIds    cmap.ConcurrentMap[string, string]
	closeNotify   <-chan struct{}
	linkFaults    metrics.Meter
	circuitFaults metrics.Meter
}

type FaultReceiver interface {
	Report(circuitId string, ctrlId string)
	NotifyInvalidLink(linkId string)
}

func NewFaulter(routerEnv env.RouterEnv, interval time.Duration) *Faulter {
	f := &Faulter{
		ctrls:         routerEnv.GetNetworkControllers(),
		interval:      interval,
		circuitIds:    cmap.New[string](),
		closeNotify:   routerEnv.GetCloseNotify(),
		linkFaults:    routerEnv.GetMetricsRegistry().Meter("faults.link"),
		circuitFaults: routerEnv.GetMetricsRegistry().Meter("faults.circuit"),
	}

	if interval > 0 {
		go f.run()
	}

	return f
}

func (self *Faulter) Report(circuitId string, ctrlId string) {
	self.circuitFaults.Mark(1)
	if self.interval > 0 {
		self.circuitIds.Set(circuitId, ctrlId)
	}
}

func (self *Faulter) NotifyInvalidLink(linkId string) {
	self.ctrls.ForEach(func(ctrlId string, ch channel.Channel) {
		fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_LinkFault, Id: linkId}
		if err := protobufs.MarshalTyped(fault).WithTimeout(self.ctrls.DefaultRequestTimeout()).Send(ch); err != nil {
			faultLog.Error("failed to notify of invalid link",
				"error", err,
				"ctrlId", ctrlId,
				"linkId", linkId,
			)
		}
	})
	self.linkFaults.Mark(1)
}

func (self *Faulter) run() {
	faultLog.Info("started")
	defer faultLog.Error("exited")

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

				if ctrlId != "" {
					log := faultLog.With("ctrlId", ctrlId)
					ch := self.ctrls.GetChannel(ctrlId)
					if ch == nil {
						log.Error("unable to report circuit fault, no control channel for controller")
						continue
					}

					fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_ForwardFault, Id: circuitIds}
					if err := protobufs.MarshalTyped(fault).Send(ch); err == nil {
						log.Debug("reported forwarding faults", "circuitCount", len(workload))
					} else {
						log.Error("error sending fault report", "error", err)
					}
				} else { // send to all controllers
					fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_UnknownOwnerForwardFault, Id: circuitIds}

					self.ctrls.ForEach(func(ctrlId string, ch channel.Channel) {
						log := faultLog.With("ctrlId", ctrlId)
						if err := protobufs.MarshalTyped(fault).Send(ch); err == nil {
							log.Debug("reported forwarding faults", "circuitCount", len(workload))
						} else {
							log.Error("error sending fault report", "error", err)
						}
					})
				}
			}

		case <-self.closeNotify:
			return
		}
	}
}
