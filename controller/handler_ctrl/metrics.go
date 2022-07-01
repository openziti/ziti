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
	"google.golang.org/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/metrics"
	"github.com/openziti/channel"
	"github.com/openziti/metrics/metrics_pb"
)

type metricsHandler struct {
	metrics.MessageHandler
}

func newMetricsHandler(network *network.Network) *metricsHandler {
	return &metricsHandler{metrics.NewDispatchWrapper(network.GetEventDispatcher().Dispatch)}
}

func (h *metricsHandler) ContentType() int32 {
	return int32(metrics_pb.ContentType_MetricsType)
}

func (h *metricsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	metricsMsg := &metrics_pb.MetricsMessage{}
	if err := proto.Unmarshal(msg.Body, metricsMsg); err == nil {
		go h.AcceptMetrics(metricsMsg)
	} else {
		pfxlog.ContextLogger(ch.Label()).Errorf("unexpected error (%s)", err)
	}
}
