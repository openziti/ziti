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
	log := pfxlog.Logger()

	reporter.msgs = append(reporter.msgs, message)

	ch := reporter.ctrls.AnyCtrlChannel()
	if ch != nil {
		for len(reporter.msgs) > 0 {
			message = reporter.msgs[0]
			bytes, err := proto.Marshal(message)
			if err != nil {
				log.Errorf("Failed to encode metrics message: %v", err)
				return
			}

			chMsg := channel.NewMessage(int32(metrics_pb.ContentType_MetricsType), bytes)

			if err = ch.Send(chMsg); err != nil {
				log.WithError(err).Error("failed to send metrics message")
				return
			}

			reporter.msgs[0] = nil
			reporter.msgs = reporter.msgs[1:]
			log.Trace("reported metrics to fabric controller")
		}
	} else {
		log.Error("no controllers available to submit metrics to")
	}
}

// NewControllersReporter creates a metrics handler which sends metrics messages to the controllers
func NewControllersReporter(ctrls env.NetworkControllers) metrics.Handler {
	return &controllersReporter{
		ctrls: ctrls,
	}
}
