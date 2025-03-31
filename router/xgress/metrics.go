package xgress

import (
	"github.com/openziti/metrics"
	"sync/atomic"
	"time"
)

type Metrics interface {
	MarkAckReceived()
	MarkPayloadDropped()
	MarkDuplicateAck()
	MarkDuplicatePayload()
	BufferBlockedByLocalWindow()
	BufferUnblockedByLocalWindow()
	BufferBlockedByRemoteWindow()
	BufferUnblockedByRemoteWindow()

	PayloadWritten(duration time.Duration)
	BufferUnblocked(duration time.Duration)

	SendPayloadBuffered(payloadSize int64)
	SendPayloadDelivered(payloadSize int64)
}

type metricsImpl struct {
	ackRxMeter           metrics.Meter
	droppedPayloadsMeter metrics.Meter

	payloadWriteTimer      metrics.Timer
	duplicateAcksMeter     metrics.Meter
	duplicatePayloadsMeter metrics.Meter

	buffersBlockedByLocalWindow  int64
	buffersBlockedByRemoteWindow int64
	outstandingPayloads          int64
	outstandingPayloadBytes      int64

	buffersBlockedByLocalWindowMeter  metrics.Meter
	buffersBlockedByRemoteWindowMeter metrics.Meter

	bufferBlockedTime metrics.Timer
}

func (self *metricsImpl) SendPayloadBuffered(payloadSize int64) {
	atomic.AddInt64(&self.outstandingPayloads, 1)
	atomic.AddInt64(&self.outstandingPayloadBytes, payloadSize)
}

func (self *metricsImpl) SendPayloadDelivered(payloadSize int64) {
	atomic.AddInt64(&self.outstandingPayloads, -1)
	atomic.AddInt64(&self.outstandingPayloadBytes, -payloadSize)
}

func (self *metricsImpl) MarkAckReceived() {
	self.ackRxMeter.Mark(1)
}

func (self *metricsImpl) MarkPayloadDropped() {
	self.droppedPayloadsMeter.Mark(1)
}

func (self *metricsImpl) MarkDuplicateAck() {
	self.duplicateAcksMeter.Mark(1)
}

func (self *metricsImpl) MarkDuplicatePayload() {
	self.duplicatePayloadsMeter.Mark(1)
}

func (self *metricsImpl) BufferBlockedByLocalWindow() {
	atomic.AddInt64(&self.buffersBlockedByLocalWindow, 1)
	self.buffersBlockedByLocalWindowMeter.Mark(1)
}

func (self *metricsImpl) BufferUnblockedByLocalWindow() {
	atomic.AddInt64(&self.buffersBlockedByLocalWindow, -1)
}

func (self *metricsImpl) BufferBlockedByRemoteWindow() {
	atomic.AddInt64(&self.buffersBlockedByRemoteWindow, 1)
	self.buffersBlockedByRemoteWindowMeter.Mark(1)
}

func (self *metricsImpl) BufferUnblockedByRemoteWindow() {
	atomic.AddInt64(&self.buffersBlockedByRemoteWindow, -1)
}

func (self *metricsImpl) PayloadWritten(duration time.Duration) {
	self.payloadWriteTimer.Update(duration)
}

func (self *metricsImpl) BufferUnblocked(duration time.Duration) {
	self.bufferBlockedTime.Update(duration)
}

func NewMetrics(registry metrics.Registry) Metrics {
	impl := &metricsImpl{
		droppedPayloadsMeter:              registry.Meter("xgress.dropped_payloads"),
		ackRxMeter:                        registry.Meter("xgress.rx.acks"),
		payloadWriteTimer:                 registry.Timer("xgress.tx_write_time"),
		duplicateAcksMeter:                registry.Meter("xgress.ack_duplicates"),
		duplicatePayloadsMeter:            registry.Meter("xgress.payload_duplicates"),
		buffersBlockedByLocalWindowMeter:  registry.Meter("xgress.blocked_by_local_window_rate"),
		buffersBlockedByRemoteWindowMeter: registry.Meter("xgress.blocked_by_remote_window_rate"),
		bufferBlockedTime:                 registry.Timer("xgress.blocked_time"),
	}

	registry.FuncGauge("xgress.blocked_by_local_window", func() int64 {
		return atomic.LoadInt64(&impl.buffersBlockedByLocalWindow)
	})

	registry.FuncGauge("xgress.blocked_by_remote_window", func() int64 {
		return atomic.LoadInt64(&impl.buffersBlockedByRemoteWindow)
	})

	registry.FuncGauge("xgress.tx_unacked_payloads", func() int64 {
		return atomic.LoadInt64(&impl.outstandingPayloads)
	})

	registry.FuncGauge("xgress.tx_unacked_payload_bytes", func() int64 {
		return atomic.LoadInt64(&impl.outstandingPayloadBytes)
	})

	return impl
}
