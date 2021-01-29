package metrics

import (
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/metrics"
)

// NewCtrlChannelPeekHandler creates a channel PeekHandler which tracks message rate and message size distribution
func NewCtrlChannelPeekHandler(routerId string, registry metrics.Registry) channel2.PeekHandler {
	txBytesMeter := registry.Meter("ctrl." + routerId + ".tx.bytesrate")
	txMsgMeter := registry.Meter("ctrl." + routerId + ".tx.msgrate")
	txMsgSizeHistogram := registry.Histogram("ctrl." + routerId + ".tx.msgsize")
	rxBytesMeter := registry.Meter("ctrl." + routerId + ".rx.bytesrate")
	rxMsgMeter := registry.Meter("ctrl." + routerId + ".rx.msgrate")
	rxMsgSizeHistogram := registry.Histogram("ctrl." + routerId + ".rx.msgsize")

	closeHook := func() {
		txBytesMeter.Dispose()
		txMsgMeter.Dispose()
		txMsgSizeHistogram.Dispose()
		rxBytesMeter.Dispose()
		rxMsgMeter.Dispose()
		rxMsgSizeHistogram.Dispose()
	}

	return &ctrlChannelPeekHandler{
		txBytesMeter:       txBytesMeter,
		txMsgMeter:         txMsgMeter,
		txMsgSizeHistogram: txMsgSizeHistogram,
		rxBytesMeter:       rxBytesMeter,
		rxMsgMeter:         rxMsgMeter,
		rxMsgSizeHistogram: rxMsgSizeHistogram,
		closeHook:          closeHook,
	}
}

type ctrlChannelPeekHandler struct {
	txBytesMeter       metrics.Meter
	txMsgMeter         metrics.Meter
	rxBytesMeter       metrics.Meter
	rxMsgMeter         metrics.Meter
	txMsgSizeHistogram metrics.Histogram
	rxMsgSizeHistogram metrics.Histogram

	closeHook func()
}

func (h *ctrlChannelPeekHandler) Connect(channel2.Channel, string) {
}

func (h *ctrlChannelPeekHandler) Rx(msg *channel2.Message, _ channel2.Channel) {
	msgSize := int64(len(msg.Body))
	h.rxBytesMeter.Mark(msgSize)
	h.rxMsgMeter.Mark(1)
	h.rxMsgSizeHistogram.Update(msgSize)
}

func (h *ctrlChannelPeekHandler) Tx(msg *channel2.Message, _ channel2.Channel) {
	msgSize := int64(len(msg.Body))
	h.txBytesMeter.Mark(msgSize)
	h.txMsgMeter.Mark(1)
	h.txMsgSizeHistogram.Update(msgSize)
}

func (h *ctrlChannelPeekHandler) Close(channel2.Channel) {
	if h.closeHook != nil {
		h.closeHook()
	}
}
