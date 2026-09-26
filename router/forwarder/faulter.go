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

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/sirupsen/logrus"
)

type Faulter struct {
	ctrls         env.NetworkControllers
	interval      time.Duration
	circuitIds    cmap.ConcurrentMap[string, string]
	closeNotify   <-chan struct{}
	linkFaults    metrics.Meter
	circuitFaults metrics.Meter

	// Endpoint faults are delivered per controller, separately from the batched forwarding
	// faults above: they are reported when a circuit's endpoint goes away rather than when
	// traffic arrives for an unknown circuit, so nothing re-reports them and each needs its own
	// retry. Senders are created on demand, since a router does not necessarily hold circuits
	// for every controller.
	endpointFaultSenders   cmap.ConcurrentMap[string, *endpointFaultSender]
	endpointFaultRetention time.Duration
	endpointFaultsSent     metrics.Meter
	endpointFaultsExpired  metrics.Meter
}

type FaultReceiver interface {
	Report(circuitId string, ctrlId string)
	NotifyInvalidLink(linkId string)

	// ReportEndpointFault reports that a circuit's ingress or egress endpoint on this router has
	// gone away, so the controller can remove the circuit. It does not block and does not wait
	// for the controller; delivery, retry and eventual expiry are the faulter's.
	ReportEndpointFault(circuitId string, ctrlId string, subject ctrl_pb.FaultSubject)
}

func NewFaulter(routerEnv env.RouterEnv, interval time.Duration) *Faulter {
	f := &Faulter{
		ctrls:         routerEnv.GetNetworkControllers(),
		interval:      interval,
		circuitIds:    cmap.New[string](),
		closeNotify:   routerEnv.GetCloseNotify(),
		linkFaults:    routerEnv.GetMetricsRegistry().Meter("faults.link"),
		circuitFaults: routerEnv.GetMetricsRegistry().Meter("faults.circuit"),

		endpointFaultSenders:   cmap.New[*endpointFaultSender](),
		endpointFaultRetention: routerEnv.GetConfig().Forwarder.EndpointFaultRetention,
		endpointFaultsSent:     routerEnv.GetMetricsRegistry().Meter("faults.circuit.endpoint"),
		endpointFaultsExpired:  routerEnv.GetMetricsRegistry().Meter("faults.circuit.endpoint.expired"),
	}

	// Endpoint faults are delivered regardless of the forwarding fault interval: unlike a
	// forwarding fault, which the forwarder re-reports while traffic keeps arriving, an
	// unreported endpoint fault leaves the controller holding a circuit forever.
	routerEnv.GetNetworkControllers().AddChangeListener(env.CtrlEventListenerFunc(f.notifyOfCtrlEvent))

	if interval > 0 {
		go f.run()
	}

	return f
}

// notifyOfCtrlEvent keeps the per-controller endpoint fault senders in step with the cluster. A
// reconnecting controller is woken so anything held for it goes out immediately rather than
// waiting for the sweep; a removed controller's senders are shut down, abandoning what they hold,
// since a controller that has left the cluster has no circuits to clean up.
func (self *Faulter) notifyOfCtrlEvent(event env.CtrlEvent) {
	if event.Controller == nil {
		return
	}

	ctrlId := event.Controller.Channel().Id()

	switch event.Type {
	case env.ControllerReconnected, env.ControllerAdded:
		if sender, found := self.endpointFaultSenders.Get(ctrlId); found {
			sender.wake()
		}
	case env.ControllerRemoved:
		if sender, found := self.endpointFaultSenders.Get(ctrlId); found {
			self.endpointFaultSenders.Remove(ctrlId)
			sender.shutdown()
		}
	}
}

func (self *Faulter) ReportEndpointFault(circuitId string, ctrlId string, subject ctrl_pb.FaultSubject) {
	if ctrlId == "" {
		pfxlog.Logger().
			WithField("circuitId", circuitId).
			WithField("subject", subject.String()).
			Error("cannot report circuit endpoint fault, no controller for circuit")
		return
	}

	sender := self.endpointFaultSenders.Upsert(ctrlId, nil, func(exists bool, current *endpointFaultSender, _ *endpointFaultSender) *endpointFaultSender {
		if current != nil {
			return current
		}
		return newEndpointFaultSender(ctrlId, self)
	})

	self.circuitFaults.Mark(1)
	sender.report(endpointFaultKey{circuitId: circuitId, subject: subject})
}

func (self *Faulter) Report(circuitId string, ctrlId string) {
	self.circuitFaults.Mark(1)
	if self.interval > 0 {
		self.circuitIds.Set(circuitId, ctrlId)
	}
}

func (self *Faulter) NotifyInvalidLink(linkId string) {
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
	self.linkFaults.Mark(1)
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

				if ctrlId != "" {
					log := pfxlog.Logger().WithField("ctrlId", ctrlId)
					ch := self.ctrls.GetChannel(ctrlId)
					if ch == nil {
						log.Error("unable to report circuit fault, no control channel for controller")
						continue
					}

					fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_ForwardFault, Id: circuitIds}
					if err := protobufs.MarshalTyped(fault).Send(ch); err == nil {
						log.WithField("circuitCount", len(workload)).Debug("reported forwarding faults")
					} else {
						log.WithError(err).Error("error sending fault report")
					}
				} else { // send to all controllers
					fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_UnknownOwnerForwardFault, Id: circuitIds}

					self.ctrls.ForEach(func(ctrlId string, ch channel.Channel) {
						log := pfxlog.Logger().WithField("ctrlId", ctrlId)
						if err := protobufs.MarshalTyped(fault).Send(ch); err == nil {
							log.WithField("circuitCount", len(workload)).Debug("reported forwarding faults")
						} else {
							log.WithError(err).Error("error sending fault report")
						}
					})
				}
			}

		case <-self.closeNotify:
			return
		}
	}
}
