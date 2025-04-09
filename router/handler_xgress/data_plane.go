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

package handler_xgress

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/router/forwarder"
	"github.com/openziti/ziti/router/xgress"
	"time"
)

type dataPlaneAdapter struct {
	acker           xgress.AckSender
	forwarder       *forwarder.Forwarder
	retransmitter   *xgress.Retransmitter
	payloadIngester *xgress.PayloadIngester
	metrics         xgress.Metrics
}

type DataPlaneAdapterConfig struct {
	Acker           xgress.AckSender
	Forwarder       *forwarder.Forwarder
	Retransmitter   *xgress.Retransmitter
	PayloadIngester *xgress.PayloadIngester
	Metrics         xgress.Metrics
}

func NewXgressDataPlaneAdapter(cfg DataPlaneAdapterConfig) xgress.DataPlaneAdapter {
	return &dataPlaneAdapter{
		acker:           cfg.Acker,
		forwarder:       cfg.Forwarder,
		retransmitter:   cfg.Retransmitter,
		payloadIngester: cfg.PayloadIngester,
		metrics:         cfg.Metrics,
	}
}

func (adapter *dataPlaneAdapter) ForwardPayload(payload *xgress.Payload, x *xgress.Xgress) {
	for {
		if err := adapter.forwarder.ForwardPayload(x.Address(), payload, time.Second); err != nil {
			if !channel.IsTimeout(err) {
				pfxlog.ContextLogger(x.Label()).WithFields(payload.GetLoggerFields()).WithError(err).Error("unable to forward payload")
				adapter.forwarder.ReportForwardingFault(payload.CircuitId, x.CtrlId())
				return
			}
		} else {
			return
		}
	}
}

func (adapter *dataPlaneAdapter) RetransmitPayload(srcAddr xgress.Address, payload *xgress.Payload) error {
	return adapter.forwarder.RetransmitPayload(srcAddr, payload)
}

func (adapter *dataPlaneAdapter) ForwardControlMessage(control *xgress.Control, x *xgress.Xgress) {
	if err := adapter.forwarder.ForwardControl(x.Address(), control); err != nil {
		pfxlog.ContextLogger(x.Label()).WithFields(control.GetLoggerFields()).WithError(err).Error("unable to forward control")
	}
}

func (adapter *dataPlaneAdapter) ForwardAcknowledgement(ack *xgress.Acknowledgement, address xgress.Address) {
	adapter.acker.SendAck(ack, address)
}

func (adapter *dataPlaneAdapter) GetRetransmitter() *xgress.Retransmitter {
	return adapter.retransmitter
}

func (adapter *dataPlaneAdapter) GetPayloadIngester() *xgress.PayloadIngester {
	return adapter.payloadIngester
}

func (adapter *dataPlaneAdapter) GetMetrics() xgress.Metrics {
	return adapter.metrics
}
