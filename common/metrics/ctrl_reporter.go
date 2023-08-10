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

package metrics

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/router/env"
	"github.com/openziti/metrics"
	"github.com/openziti/metrics/metrics_pb"
	"google.golang.org/protobuf/proto"
)

type controllersReporter struct {
	ctrls env.NetworkControllers
	msgs  []*metrics_pb.MetricsMessage
}

func (reporter *controllersReporter) AcceptMetrics(message *metrics_pb.MetricsMessage) {
	reporter.msgs = append(reporter.msgs, message)

	for len(reporter.msgs) > 0 {
		message = reporter.msgs[0]

		successfulSend := false

		for ctrlId, ctrl := range reporter.ctrls.GetAll() {
			log := pfxlog.Logger().WithField("ctrlId", ctrlId)

			// once we've had a successful send, tell other controllers not to propagate the event
			message.DoNotPropagate = successfulSend

			bytes, err := proto.Marshal(message)
			if err != nil {
				log.WithError(err).Error("Failed to encode metrics message")

				// drop message, since it's invalid somehow
				reporter.msgs[0] = nil
				reporter.msgs = reporter.msgs[1:]
				break
			}

			chMsg := channel.NewMessage(int32(metrics_pb.ContentType_MetricsType), bytes)

			if err = chMsg.WithTimeout(reporter.ctrls.DefaultRequestTimeout()).SendAndWaitForWire(ctrl.Channel()); err != nil {
				log.WithError(err).Error("failed to send metrics message")
			} else {
				log.Trace("reported metrics to fabric controller")

				// after first successful send, remove the message from the queue
				if !successfulSend {
					reporter.msgs[0] = nil
					reporter.msgs = reporter.msgs[1:]
					successfulSend = true
				}
			}
		}
	}
}

// NewControllersReporter creates a metrics handler which sends metrics messages to the controllers
func NewControllersReporter(ctrls env.NetworkControllers) metrics.Handler {
	return &controllersReporter{
		ctrls: ctrls,
	}
}
