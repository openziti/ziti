package xgress_test

import (
	"time"

	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
)

type mockFaulter struct{}

func (m mockFaulter) ReportForwardingFault(circuitId string, ctrlId string) {
}

type noopMetrics struct{}

func (n noopMetrics) MarkAckReceived()               {}
func (n noopMetrics) MarkPayloadDropped()            {}
func (n noopMetrics) MarkDuplicateAck()              {}
func (n noopMetrics) MarkDuplicatePayload()          {}
func (n noopMetrics) BufferBlockedByLocalWindow()    {}
func (n noopMetrics) BufferUnblockedByLocalWindow()  {}
func (n noopMetrics) BufferBlockedByRemoteWindow()   {}
func (n noopMetrics) BufferUnblockedByRemoteWindow() {}
func (n noopMetrics) PayloadWritten(time.Duration)   {}
func (n noopMetrics) BufferUnblocked(time.Duration)  {}
func (n noopMetrics) SendPayloadBuffered(int64)      {}
func (n noopMetrics) SendPayloadDelivered(int64)     {}

type MockEnv struct {
	retransmitter   *xgress.Retransmitter
	payloadIngester *xgress.PayloadIngester
	metrics         xgress.Metrics
}

func NewMockEnv() *MockEnv {
	closeNotify := make(chan struct{})
	metricsRegistry := metrics.NewRegistry("test", nil)
	return &MockEnv{
		retransmitter:   xgress.NewRetransmitter(mockFaulter{}, metricsRegistry, closeNotify),
		payloadIngester: xgress.NewPayloadIngester(closeNotify),
		metrics:         noopMetrics{},
	}
}

func (self *MockEnv) GetRetransmitter() *xgress.Retransmitter {
	return self.retransmitter
}

func (self *MockEnv) GetPayloadIngester() *xgress.PayloadIngester {
	return self.payloadIngester
}

func (self *MockEnv) GetMetrics() xgress.Metrics {
	return self.metrics
}
