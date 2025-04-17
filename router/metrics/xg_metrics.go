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
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/router/env"
	"time"
)

func NewXgressMetrics(registry metrics.UsageRegistry) *XgressMetrics {
	ingressTxBytesMeter := registry.Meter("ingress.tx.bytesrate")
	ingressTxMsgMeter := registry.Meter("ingress.tx.msgrate")
	ingressRxBytesMeter := registry.Meter("ingress.rx.bytesrate")
	ingressRxMsgMeter := registry.Meter("ingress.rx.msgrate")
	egressTxBytesMeter := registry.Meter("egress.tx.bytesrate")
	egressTxMsgMeter := registry.Meter("egress.tx.msgrate")
	egressRxBytesMeter := registry.Meter("egress.rx.bytesrate")
	egressRxMsgMeter := registry.Meter("egress.rx.msgrate")

	ingressTxMsgSizeHistogram := registry.Histogram("ingress.tx.msgsize")
	ingressRxMsgSizeHistogram := registry.Histogram("ingress.rx.msgsize")
	egressTxMsgSizeHistogram := registry.Histogram("egress.tx.msgsize")
	egressRxMsgSizeHistogram := registry.Histogram("egress.rx.msgsize")

	return &XgressMetrics{
		ingressTxBytesMeter: ingressTxBytesMeter,
		ingressTxMsgMeter:   ingressTxMsgMeter,
		ingressRxBytesMeter: ingressRxBytesMeter,
		ingressRxMsgMeter:   ingressRxMsgMeter,
		egressTxBytesMeter:  egressTxBytesMeter,
		egressTxMsgMeter:    egressTxMsgMeter,
		egressRxBytesMeter:  egressRxBytesMeter,
		egressRxMsgMeter:    egressRxMsgMeter,

		ingressTxMsgSizeHistogram: ingressTxMsgSizeHistogram,
		ingressRxMsgSizeHistogram: ingressRxMsgSizeHistogram,
		egressTxMsgSizeHistogram:  egressTxMsgSizeHistogram,
		egressRxMsgSizeHistogram:  egressRxMsgSizeHistogram,

		usageCounter: registry.UsageCounter("usage", env.IntervalSize),
	}
}

type XgressMetrics struct {
	ingressTxBytesMeter metrics.Meter
	ingressTxMsgMeter   metrics.Meter
	ingressRxBytesMeter metrics.Meter
	ingressRxMsgMeter   metrics.Meter
	egressTxBytesMeter  metrics.Meter
	egressTxMsgMeter    metrics.Meter
	egressRxBytesMeter  metrics.Meter
	egressRxMsgMeter    metrics.Meter

	ingressTxMsgSizeHistogram metrics.Histogram
	ingressRxMsgSizeHistogram metrics.Histogram
	egressTxMsgSizeHistogram  metrics.Histogram
	egressRxMsgSizeHistogram  metrics.Histogram

	usageCounter metrics.UsageCounter
}

func (handler *XgressMetrics) Rx(source metrics.UsageSource, originator xgress.Originator, payload *xgress.Payload) {
	msgSize := int64(len(payload.Data))
	if originator == xgress.Initiator {
		handler.usageCounter.Update(source, "ingress.rx", time.Now(), uint64(msgSize))
		handler.ingressRxMsgMeter.Mark(1)
		handler.ingressRxBytesMeter.Mark(msgSize)
		handler.ingressRxMsgSizeHistogram.Update(msgSize)
	} else {
		handler.usageCounter.Update(source, "egress.rx", time.Now(), uint64(msgSize))
		handler.egressRxMsgMeter.Mark(1)
		handler.egressRxBytesMeter.Mark(msgSize)
		handler.egressRxMsgSizeHistogram.Update(msgSize)
	}
}

func (handler *XgressMetrics) Tx(source metrics.UsageSource, originator xgress.Originator, payload *xgress.Payload) {
	msgSize := int64(len(payload.Data))
	if originator == xgress.Initiator {
		handler.usageCounter.Update(source, "ingress.tx", time.Now(), uint64(msgSize))

		handler.ingressTxMsgMeter.Mark(1)
		handler.ingressTxBytesMeter.Mark(msgSize)
		handler.ingressTxMsgSizeHistogram.Update(msgSize)
	} else {
		handler.usageCounter.Update(source, "egress.tx", time.Now(), uint64(msgSize))
		handler.egressTxMsgMeter.Mark(1)
		handler.egressTxBytesMeter.Mark(msgSize)
		handler.egressTxMsgSizeHistogram.Update(msgSize)
	}
}
