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
	"github.com/openziti/ziti/v2/common/servermetrics"
	"github.com/openziti/ziti/v2/common/servermetrics/metrics_pb"
	"github.com/openziti/ziti/v2/router/env"
	"google.golang.org/protobuf/proto"
)

type controllersReporter struct {
	ctrls      env.NetworkControllers
	maxBacklog int
	backlog    []*metrics_pb.MetricsMessage
}

// AcceptMetrics queues message behind any undelivered ones and delivers the queue in order. It stops at
// the first message no controller accepts, leaving it and the rest queued for the next call, so it never
// blocks longer than one send timeout per controller per message. When the queue exceeds its bound, the
// oldest messages are dropped. Not safe for concurrent use.
func (reporter *controllersReporter) AcceptMetrics(message *metrics_pb.MetricsMessage) {
	reporter.backlog = append(reporter.backlog, message)

	if excess := len(reporter.backlog) - reporter.maxBacklog; excess > 0 {
		pfxlog.Logger().WithField("dropped", excess).Warn("metrics backlog full, dropping oldest undelivered messages")
		clear(reporter.backlog[:excess])
		reporter.backlog = reporter.backlog[excess:]
	}

	for len(reporter.backlog) > 0 {
		if !reporter.deliver(reporter.backlog[0]) {
			pfxlog.Logger().WithField("backlog", len(reporter.backlog)).
				Warn("no controller accepted metrics message, retrying on next report")
			return
		}
		reporter.backlog[0] = nil
		reporter.backlog = reporter.backlog[1:]
	}
}

// deliver sends message to every registered controller, marking it DoNotPropagate once one has accepted
// it. It reports whether the message is finished with: accepted by at least one controller, or
// unencodable and so dropped.
func (reporter *controllersReporter) deliver(message *metrics_pb.MetricsMessage) bool {
	delivered := false

	for ctrlId, ctrl := range reporter.ctrls.GetAll() {
		log := pfxlog.Logger().WithField("ctrlId", ctrlId)

		message.DoNotPropagate = delivered

		bytes, err := proto.Marshal(message)
		if err != nil {
			log.WithError(err).Error("failed to encode metrics message, dropping it")
			return true
		}

		chMsg := channel.NewMessage(int32(metrics_pb.ContentType_MetricsType), bytes)

		if err = chMsg.WithTimeout(reporter.ctrls.DefaultRequestTimeout()).SendAndWaitForWire(ctrl.Channel()); err != nil {
			log.WithError(err).Error("failed to send metrics message")
		} else {
			log.Trace("reported metrics to fabric controller")
			delivered = true
		}
	}

	return delivered
}

// NewControllersReporter creates a metrics handler which sends metrics messages to the controllers,
// holding up to maxBacklog undelivered messages while no controller accepts them.
func NewControllersReporter(ctrls env.NetworkControllers, maxBacklog int) servermetrics.Handler {
	return &controllersReporter{
		ctrls:      ctrls,
		maxBacklog: max(maxBacklog, 1),
	}
}
