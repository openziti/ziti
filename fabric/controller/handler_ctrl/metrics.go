/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-fabric/fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/fabric/metrics"
	"github.com/netfoundry/ziti-fabric/fabric/pb/ctrl_pb"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
)

type metricsHandler struct {
	metrics.EventController
}

func newMetricsHandler(network *network.Network) *metricsHandler {
	return &metricsHandler{network.GetMetricsEventController()}
}

func (h *metricsHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_MetricsType)
}

func (h *metricsHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	metricsMsg := &ctrl_pb.MetricsMessage{}
	if err := proto.Unmarshal(msg.Body, metricsMsg); err == nil {
		h.AcceptMetrics(metricsMsg)
	} else {
		pfxlog.ContextLogger(ch.Label()).Errorf("unexpected error (%s)", err)
	}
}