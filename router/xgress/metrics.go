package xgress

import (
	"github.com/openziti/foundation/metrics"
	"sync/atomic"
)

var ackTxMeter metrics.Meter
var ackRxMeter metrics.Meter
var droppedPayloadsMeter metrics.Meter
var retransmissions metrics.Meter
var retransmissionFailures metrics.Meter

var ackFailures metrics.Meter
var rttHistogram metrics.Histogram
var txBufferSizeHistogram metrics.Histogram
var localRecvBufferSizeBytesHistogram metrics.Histogram
var localRecvBufferSizeMsgsHistogram metrics.Histogram
var remoteRecvBufferSizeHistogram metrics.Histogram
var payloadWriteTimer metrics.Timer
var duplicateAcksMeter metrics.Meter
var txWindowSize metrics.Histogram

var buffersBlockedByLocalWindow int64
var buffersBlockedByRemoteWindow int64
var outstandingPayloads int64
var outstandingPayloadBytes int64

func InitMetrics(registry metrics.UsageRegistry) {
	droppedPayloadsMeter = registry.Meter("xgress.dropped_payloads")
	retransmissions = registry.Meter("xgress.retransmissions")
	retransmissionFailures = registry.Meter("xgress.retransmission_failures")
	ackRxMeter = registry.Meter("xgress.rx.acks")
	ackTxMeter = registry.Meter("xgress.tx.acks")
	ackFailures = registry.Meter("xgress.ack_failures")
	rttHistogram = registry.Histogram("xgress.rtt")
	txBufferSizeHistogram = registry.Histogram("xgress.tx_buffer_size")
	localRecvBufferSizeBytesHistogram = registry.Histogram("xgress.local.rx_buffer_bytes_size")
	localRecvBufferSizeMsgsHistogram = registry.Histogram("xgress.local.rx_buffer_msgs_size")
	remoteRecvBufferSizeHistogram = registry.Histogram("xgress.remote.rx_buffer_size")
	payloadWriteTimer = registry.Timer("xgress.tx_write_time")
	duplicateAcksMeter = registry.Meter("xgress.ack_duplicates")
	txWindowSize = registry.Histogram("xgress.tx_window_size")

	registry.FuncGauge("xgress.blocked_by_local_window", func() int64 {
		return atomic.LoadInt64(&buffersBlockedByLocalWindow)
	})

	registry.FuncGauge("xgress.blocked_by_remote_window", func() int64 {
		return atomic.LoadInt64(&buffersBlockedByRemoteWindow)
	})

	registry.FuncGauge("xgress.tx_unacked_payloads", func() int64 {
		return atomic.LoadInt64(&outstandingPayloads)
	})

	registry.FuncGauge("xgress.tx_unacked_payload_bytes", func() int64 {
		return atomic.LoadInt64(&outstandingPayloadBytes)
	})
}
