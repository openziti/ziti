package metrics

import (
	"github.com/openziti/channel"
	"github.com/openziti/metrics"
)

// NewCtrlChannelPeekHandler creates a channel PeekHandler which tracks message rate and message size distribution
func NewCtrlChannelPeekHandler(routerId string, registry metrics.Registry) channel.PeekHandler {
	txBytesMeter := registry.Meter("ctrl.tx.bytesrate:" + routerId)
	txMsgMeter := registry.Meter("ctrl.tx.msgrate:" + routerId)
	txMsgSizeHistogram := registry.Histogram("ctrl.tx.msgsize:" + routerId)
	rxBytesMeter := registry.Meter("ctrl.rx.bytesrate:" + routerId)
	rxMsgMeter := registry.Meter("ctrl.rx.msgrate:" + routerId)
	rxMsgSizeHistogram := registry.Histogram("ctrl.rx.msgsize:" + routerId)

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

func (h *ctrlChannelPeekHandler) Connect(channel.Channel, string) {
}

func (h *ctrlChannelPeekHandler) Rx(msg *channel.Message, _ channel.Channel) {
	msgSize := int64(len(msg.Body))
	h.rxBytesMeter.Mark(msgSize)
	h.rxMsgMeter.Mark(1)
	h.rxMsgSizeHistogram.Update(msgSize)
}

func (h *ctrlChannelPeekHandler) Tx(msg *channel.Message, _ channel.Channel) {
	msgSize := int64(len(msg.Body))
	h.txBytesMeter.Mark(msgSize)
	h.txMsgMeter.Mark(1)
	h.txMsgSizeHistogram.Update(msgSize)
}

func (h *ctrlChannelPeekHandler) Close(channel.Channel) {
	if h.closeHook != nil {
		h.closeHook()
	}
}
