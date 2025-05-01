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
	"github.com/openziti/channel/v4"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/router/env"
	"time"
)

// NewChannelPeekHandler creates a channel PeekHandler which tracks latency, message rate and message size distribution
func NewChannelPeekHandler(linkId string, registry metrics.UsageRegistry) channel.PeekHandler {
	appTxBytesMeter := registry.Meter("fabric.tx.bytesrate")
	appTxMsgMeter := registry.Meter("fabric.tx.msgrate")
	appTxMsgSizeHistogram := registry.Histogram("fabric.tx.msgsize")

	appRxBytesMeter := registry.Meter("fabric.rx.bytesrate")
	appRxMsgMeter := registry.Meter("fabric.rx.msgrate")
	appRxMsgSizeHistogram := registry.Histogram("fabric.rx.msgsize")

	linkTxBytesMeter := registry.Meter("link." + linkId + ".tx.bytesrate")
	linkTxMsgMeter := registry.Meter("link." + linkId + ".tx.msgrate")
	linkTxMsgSizeHistogram := registry.Histogram("link." + linkId + ".tx.msgsize")
	linkRxBytesMeter := registry.Meter("link." + linkId + ".rx.bytesrate")
	linkRxMsgMeter := registry.Meter("link." + linkId + ".rx.msgrate")
	linkRxMsgSizeHistogram := registry.Histogram("link." + linkId + ".rx.msgsize")

	usageCounter := registry.UsageCounter("fabricUsage", env.IntervalSize)

	return &channelPeekHandler{
		appTxBytesMeter:        appTxBytesMeter,
		appTxMsgMeter:          appTxMsgMeter,
		appTxMsgSizeHistogram:  appTxMsgSizeHistogram,
		appRxBytesMeter:        appRxBytesMeter,
		appRxMsgMeter:          appRxMsgMeter,
		appRxMsgSizeHistogram:  appRxMsgSizeHistogram,
		linkTxBytesMeter:       linkTxBytesMeter,
		linkTxMsgMeter:         linkTxMsgMeter,
		linkTxMsgSizeHistogram: linkTxMsgSizeHistogram,
		linkRxBytesMeter:       linkRxBytesMeter,
		linkRxMsgMeter:         linkRxMsgMeter,
		linkRxMsgSizeHistogram: linkRxMsgSizeHistogram,
		usageCounter:           usageCounter,
	}
}

type channelPeekHandler struct {
	appTxBytesMeter metrics.Meter
	appTxMsgMeter   metrics.Meter
	appRxBytesMeter metrics.Meter
	appRxMsgMeter   metrics.Meter

	appTxMsgSizeHistogram metrics.Histogram
	appRxMsgSizeHistogram metrics.Histogram

	linkTxBytesMeter       metrics.Meter
	linkTxMsgMeter         metrics.Meter
	linkRxBytesMeter       metrics.Meter
	linkRxMsgMeter         metrics.Meter
	linkTxMsgSizeHistogram metrics.Histogram
	linkRxMsgSizeHistogram metrics.Histogram

	usageCounter metrics.UsageCounter
}

func (h *channelPeekHandler) Connect(channel.Channel, string) {
}

func (h *channelPeekHandler) Rx(msg *channel.Message, _ channel.Channel) {
	msgSize := int64(len(msg.Body))
	h.linkRxBytesMeter.Mark(msgSize)
	h.linkRxMsgMeter.Mark(1)
	h.linkRxMsgSizeHistogram.Update(msgSize)
	h.appRxBytesMeter.Mark(msgSize)
	h.appRxMsgMeter.Mark(1)
	h.appRxMsgSizeHistogram.Update(msgSize)

	if msg.ContentType == int32(xgress.ContentTypePayloadType) {
		circuitId, ok := msg.Headers[xgress.HeaderKeyCircuitId]
		if !ok {
			pfxlog.Logger().Error("no circuit id in payload")
		} else {
			h.usageCounter.Update(circuitUsageSource(circuitId), "fabric.rx", time.Now(), uint64(len(msg.Body)))
		}
	}
}

func (h *channelPeekHandler) Tx(msg *channel.Message, _ channel.Channel) {
	msgSize := int64(len(msg.Body))
	h.linkTxBytesMeter.Mark(msgSize)
	h.linkTxMsgMeter.Mark(1)
	h.linkTxMsgSizeHistogram.Update(msgSize)
	h.appTxBytesMeter.Mark(msgSize)
	h.appTxMsgMeter.Mark(1)
	h.appTxMsgSizeHistogram.Update(msgSize)

	if msg.ContentType == int32(xgress.ContentTypePayloadType) {
		if payload, err := xgress.UnmarshallPayload(msg); err != nil {
			pfxlog.Logger().WithError(err).Error("Failed to unmarshal payload")
		} else {
			h.usageCounter.Update(circuitUsageSource(payload.CircuitId), "fabric.tx", time.Now(), uint64(len(payload.Data)))
		}
	}
}

func (h *channelPeekHandler) Close(channel.Channel) {
	// app level metrics and usageCounter are shared across all links, so we don't dispose of them
	h.linkTxBytesMeter.Dispose()
	h.linkTxMsgMeter.Dispose()
	h.linkTxMsgSizeHistogram.Dispose()
	h.linkRxBytesMeter.Dispose()
	h.linkRxMsgMeter.Dispose()
	h.linkRxMsgSizeHistogram.Dispose()
}

// NewXgressPeekHandler creates an xgress PeekHandler which tracks message rates and histograms as well as usage
func NewXgressPeekHandler(xgressMetrics env.XgressMetrics) xgress.PeekHandler {
	return &xgressPeekHandler{
		metrics: xgressMetrics,
	}
}

type circuitUsageSource string

func (c circuitUsageSource) GetIntervalId() string {
	return string(c)
}

func (c circuitUsageSource) GetTags() map[string]string {
	return nil
}

type xgressPeekHandler struct {
	metrics env.XgressMetrics
}

func (handler *xgressPeekHandler) Rx(x *xgress.Xgress, payload *xgress.Payload) {
	handler.metrics.Rx(x, x.Originator(), payload)
}

func (handler *xgressPeekHandler) Tx(x *xgress.Xgress, payload *xgress.Payload) {
	handler.metrics.Tx(x, x.Originator(), payload)
}

func (handler *xgressPeekHandler) Close(*xgress.Xgress) {
}
