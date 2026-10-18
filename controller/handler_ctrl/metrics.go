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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/servermetrics/metrics_pb"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/network"
	"google.golang.org/protobuf/proto"
)

type metricsHandler struct {
	dispatcher event.Dispatcher
	received   metrics.Meter
}

func newMetricsHandler(network *network.Network) *metricsHandler {
	return &metricsHandler{
		dispatcher: network.GetEventDispatcher(),
		// Counts metrics messages this controller ingests. Once link latency is in
		// gossip, routers narrow the firehose to a single subscription controller,
		// so every other controller's rate should fall to zero; this meter makes
		// that per-controller ingestion drop observable.
		received: network.GetMetricsRegistry().Meter("metrics.messages.received"),
	}
}

func (h *metricsHandler) ContentType() int32 {
	return int32(metrics_pb.ContentType_MetricsType)
}

func (h *metricsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	h.received.Mark(1)
	metricsMsg := &metrics_pb.MetricsMessage{}
	if err := proto.Unmarshal(msg.Body, metricsMsg); err == nil {
		h.dispatcher.AcceptMetricsMsg(metricsMsg)
	} else {
		pfxlog.ContextLogger(ch.Label()).Errorf("unexpected error (%s)", err)
	}
}
