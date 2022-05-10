/*
	Copyright NetFoundry, Inc.

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
	"google.golang.org/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/foundation/metrics/metrics_pb"
)

type channelReporter struct {
	ch channel.Channel
}

func (reporter *channelReporter) AcceptMetrics(message *metrics_pb.MetricsMessage) {
	log := pfxlog.Logger()

	bytes, err := proto.Marshal(message)
	if err != nil {
		log.Errorf("Failed to encode metrics message: %v", err)
		return
	}

	chMsg := channel.NewMessage(int32(metrics_pb.ContentType_MetricsType), bytes)

	if err = reporter.ch.Send(chMsg); err != nil {
		log.WithError(err).Error("failed to send metrics message")
	} else {
		log.Trace("reported metrics to fabric controller")
	}
}

// NewChannelReporter creates a metrics handler which sends metrics messages out on the given channel
func NewChannelReporter(ch channel.Channel) MessageHandler {
	return &channelReporter{
		ch: ch,
	}
}
