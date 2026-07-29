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
	"context"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/openziti/ziti/v2/router/forwarder"
)

// fabricPath is the single path tag a router-side xgress uses. A router forwards into the fabric
// forwarder, which is one logical destination per circuit, so there is exactly one path and no
// pathless-hold/flush machinery (that is an SDK-side concern). It exists only to satisfy the
// DataPlaneAdapter path-tag contract; per-path RTT/loss attribution is driven by the SDK, so the
// Record* methods are no-ops.
type fabricPath struct{}

func (fabricPath) ID() string       { return "fabric" }
func (fabricPath) RecordRtt(uint16) {}
func (fabricPath) RecordLoss()      {}

// theFabricPath is the shared non-nil tag returned for every completed forward on a router. It is
// non-nil so the send buffer marks the payload sent (a router has no AddPath to flush an unsent
// payload, so it relies on mark-sent plus retransmit-on-timeout, as it always has).
var theFabricPath xgress.Path = fabricPath{}

type dataPlaneAdapter struct {
	acker           xgress.AckSender
	forwarder       *forwarder.Forwarder
	payloadIngester *xgress.PayloadIngester
	metrics         xgress.Metrics
}

type DataPlaneAdapterConfig struct {
	Acker           xgress.AckSender
	Forwarder       *forwarder.Forwarder
	PayloadIngester *xgress.PayloadIngester
	Metrics         xgress.Metrics
}

func NewXgressDataPlaneAdapter(cfg DataPlaneAdapterConfig) xgress.DataPlaneAdapter {
	return &dataPlaneAdapter{
		acker:           cfg.Acker,
		forwarder:       cfg.Forwarder,
		payloadIngester: cfg.PayloadIngester,
		metrics:         cfg.Metrics,
	}
}

func (adapter *dataPlaneAdapter) ForwardPayload(payload *xgress.Payload, x *xgress.Xgress, _ context.Context) xgress.Path {
	for {
		if err := adapter.forwarder.ForwardPayload(x.Address(), payload, time.Second); err != nil {
			if !channel.IsTimeout(err) {
				if !payload.IsCircuitEndFlagSet() && !payload.IsFlagEOFSet() {
					pfxlog.ContextLogger(x.Label()).WithFields(payload.GetLoggerFields()).WithError(err).Debug("unable to forward payload")
				}
				// No destination for the circuit (typically a transient reroute window): report the
				// fault so the controller re-splices, and mark the payload sent so it retransmits
				// once forwarding is restored, matching the router's long-standing behavior.
				adapter.forwarder.ReportForwardingFault(payload.CircuitId, x.CtrlId())
				return theFabricPath
			}
		} else {
			return theFabricPath
		}
	}
}

func (adapter *dataPlaneAdapter) RetransmitPayload(_ xgress.Path, srcAddr xgress.Address, payload *xgress.Payload) (xgress.Path, error) {
	if err := adapter.forwarder.RetransmitPayload(srcAddr, payload); err != nil {
		adapter.forwarder.ReportForwardingFault(payload.CircuitId, "")
		return nil, err
	}
	return theFabricPath, nil
}

func (adapter *dataPlaneAdapter) ForwardControlMessage(control *xgress.Control, x *xgress.Xgress) {
	if err := adapter.forwarder.ForwardControl(x.Address(), control); err != nil {
		pfxlog.ContextLogger(x.Label()).WithFields(control.GetLoggerFields()).WithError(err).Debug("unable to forward control")
	}
}

// ForwardAcknowledgement ignores the arrival-path tag: a router has a single fabric path, so there
// is no per-path arrival affinity to honor.
func (adapter *dataPlaneAdapter) ForwardAcknowledgement(ack *xgress.Acknowledgement, address xgress.Address, _ xgress.Path) {
	adapter.acker.SendAck(ack, address)
}

func (adapter *dataPlaneAdapter) GetPayloadIngester() *xgress.PayloadIngester {
	return adapter.payloadIngester
}

func (adapter *dataPlaneAdapter) GetMetrics() xgress.Metrics {
	return adapter.metrics
}
