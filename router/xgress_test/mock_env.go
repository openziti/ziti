package xgress_test

import (
	"time"

	"github.com/openziti/sdk-golang/xgress"
)

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
func (n noopMetrics) MarkRetransmission()             {}
func (n noopMetrics) MarkRetransmissionFailure()      {}

type MockEnv struct {
	payloadIngester *xgress.PayloadIngester
	metrics         xgress.Metrics
}

func NewMockEnv() *MockEnv {
	closeNotify := make(chan struct{})
	return &MockEnv{
		payloadIngester: xgress.NewPayloadIngester(closeNotify),
		metrics:         noopMetrics{},
	}
}

func (self *MockEnv) GetPayloadIngester() *xgress.PayloadIngester {
	return self.payloadIngester
}

func (self *MockEnv) GetMetrics() xgress.Metrics {
	return self.metrics
}
