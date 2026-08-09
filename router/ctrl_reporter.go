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

package router

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/servermetrics"
	"github.com/openziti/ziti/v2/common/servermetrics/metrics_pb"
	"github.com/openziti/ziti/v2/router/env"
	"google.golang.org/protobuf/proto"
)

type controllersReporter struct {
	ctrls env.NetworkControllers
	msgs  []*metrics_pb.MetricsMessage
}

func (reporter *controllersReporter) AcceptMetrics(message *metrics_pb.MetricsMessage) {
	reporter.msgs = append(reporter.msgs, message)

	// Drain the queue, but stop as soon as a message can't be delivered, leaving
	// it (and anything behind it) queued for the next report interval. This keeps
	// usage data from being dropped while avoiding a tight retry loop when no
	// controller is currently reachable.
	for len(reporter.msgs) > 0 && reporter.sendHead() {
	}
}

// sendHead tries to deliver the head of the queue. It returns true if the head
// was resolved and removed (delivered to a controller, or dropped because it
// could not be encoded), so the caller should keep draining; false if it could
// not be delivered now, in which case the message stays queued for the next
// report interval.
func (reporter *controllersReporter) sendHead() bool {
	message := reporter.msgs[0]

	// Signal whether this router publishes per-link latency over gossip, so a
	// controller knows not to also derive routing latency from this message.
	// True exactly when every connected controller is gossip-capable, the same
	// gate the gossip latency publisher uses; the histograms still travel for
	// observability.
	message.LinkLatencyInGossip = reporter.ctrls.AllControllersHaveCapability(capabilities.ControllerLinkGossip)

	successfulSend := false

	for ctrlId, ctrl := range reporter.metricsTargets(message.LinkLatencyInGossip) {
		log := pfxlog.Logger().WithField("ctrlId", ctrlId)

		// once we've had a successful send, tell other controllers not to propagate the event
		message.DoNotPropagate = successfulSend

		bytes, err := proto.Marshal(message)
		if err != nil {
			log.WithError(err).Error("failed to encode metrics message")

			// drop the message, since it's invalid somehow (unless a successful
			// send already removed it from the queue)
			if !successfulSend {
				reporter.dropHead()
			}
			return true
		}

		chMsg := channel.NewMessage(int32(metrics_pb.ContentType_MetricsType), bytes)

		if err = chMsg.WithTimeout(reporter.ctrls.DefaultRequestTimeout()).SendAndWaitForWire(ctrl.Channel()); err != nil {
			log.WithError(err).Error("failed to send metrics message")
		} else {
			log.Trace("reported metrics to fabric controller")

			// after the first successful send, remove the message from the queue
			if !successfulSend {
				reporter.dropHead()
				successfulSend = true
			}
		}
	}

	return successfulSend
}

func (reporter *controllersReporter) dropHead() {
	reporter.msgs[0] = nil
	reporter.msgs = reporter.msgs[1:]
}

// metricsTargets returns the controllers that should receive the metrics message.
// When every controller is gossip-capable, per-link latency travels over gossip
// and the metrics message is no longer routing-critical, so it is narrowed to the
// single subscription controller (the one that already receives canaries and link
// reports). Otherwise it fans out to every controller, the pre-gossip behavior, so
// each controller can derive routing latency from it. The message itself is
// unchanged either way: usage and observability metrics still travel in full.
func (reporter *controllersReporter) metricsTargets(linkLatencyInGossip bool) map[string]env.NetworkController {
	if !linkLatencyInGossip {
		return reporter.ctrls.GetAll()
	}

	if sub := reporter.ctrls.GetSubscriptionController(); sub != nil {
		return map[string]env.NetworkController{sub.Channel().Id(): sub}
	}

	return nil
}

// NewControllersReporter creates a metrics handler which sends metrics messages to the controllers
func NewControllersReporter(ctrls env.NetworkControllers) servermetrics.Handler {
	return &controllersReporter{
		ctrls: ctrls,
	}
}
