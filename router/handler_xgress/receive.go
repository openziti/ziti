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
	"github.com/openziti/channel/v3"
	"github.com/openziti/ziti/router/forwarder"
	"github.com/openziti/ziti/router/xgress"
	"time"
)

type dataPlaneHandler struct {
	acker           xgress.AckSender
	forwarder       *forwarder.Forwarder
	retransmitter   *xgress.Retransmitter
	payloadIngester *xgress.PayloadIngester
	metrics         xgress.Metrics
}

type DataPlaneHandlerConfig struct {
	Acker           xgress.AckSender
	Forwarder       *forwarder.Forwarder
	Retransmitter   *xgress.Retransmitter
	PayloadIngester *xgress.PayloadIngester
	Metrics         xgress.Metrics
}

func NewXgressDataPlaneHandler(cfg DataPlaneHandlerConfig) xgress.DataPlaneHandler {
	return &dataPlaneHandler{
		acker:           cfg.Acker,
		forwarder:       cfg.Forwarder,
		retransmitter:   cfg.Retransmitter,
		payloadIngester: cfg.PayloadIngester,
		metrics:         cfg.Metrics,
	}
}

func (xrh *dataPlaneHandler) SendPayload(payload *xgress.Payload, x *xgress.Xgress) {
	for {
		if err := xrh.forwarder.ForwardPayload(x.Address(), payload, time.Second); err != nil {
			if !channel.IsTimeout(err) {
				pfxlog.ContextLogger(x.Label()).WithFields(payload.GetLoggerFields()).WithError(err).Error("unable to forward payload")
				xrh.forwarder.ReportForwardingFault(payload.CircuitId, x.CtrlId())
				return
			}
		} else {
			return
		}
	}
}

func (xrh *dataPlaneHandler) SendControlMessage(control *xgress.Control, x *xgress.Xgress) {
	if err := xrh.forwarder.ForwardControl(x.Address(), control); err != nil {
		pfxlog.ContextLogger(x.Label()).WithFields(control.GetLoggerFields()).WithError(err).Error("unable to forward control")
	}
}

func (xrh *dataPlaneHandler) SendAcknowledgement(ack *xgress.Acknowledgement, address xgress.Address) {
	xrh.acker.SendAck(ack, address)
}

func (xrh *dataPlaneHandler) GetRetransmitter() *xgress.Retransmitter {
	return xrh.retransmitter
}

func (xrh *dataPlaneHandler) GetPayloadIngester() *xgress.PayloadIngester {
	return xrh.payloadIngester
}

func (xrh *dataPlaneHandler) GetMetrics() xgress.Metrics {
	return xrh.metrics
}
