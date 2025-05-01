package xgress_router

import (
	"github.com/ef-ds/deque"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"sync/atomic"
)

type AckForwarder interface {
	ForwardAcknowledgement(srcAddr xgress.Address, acknowledgement *xgress.Acknowledgement) error
}

type ackEntry struct {
	xgress.Address
	*xgress.Acknowledgement
}

// Note: if altering this struct, be sure to account for 64 bit alignment on 32 bit arm arch
// https://pkg.go.dev/sync/atomic#pkg-note-BUG
// https://github.com/golang/go/issues/36606
type Acker struct {
	acksQueueSize int64
	forwarder     AckForwarder
	acks          *deque.Deque
	ackIngest     chan *ackEntry
	ackSend       chan *ackEntry
	closeNotify   <-chan struct{}

	ackTxMeter  metrics.Meter
	ackFailures metrics.Meter
}

func NewAcker(forwarder AckForwarder, metrics metrics.Registry, closeNotify <-chan struct{}) *Acker {
	result := &Acker{
		forwarder:   forwarder,
		acks:        deque.New(),
		ackIngest:   make(chan *ackEntry, 16),
		ackSend:     make(chan *ackEntry, 1),
		closeNotify: closeNotify,
		ackTxMeter:  metrics.Meter("xgress.tx.acks"),
		ackFailures: metrics.Meter("xgress.ack_failures"),
	}

	go result.ackIngester()
	go result.ackSender()

	metrics.FuncGauge("xgress.acks.queue_size", func() int64 {
		return atomic.LoadInt64(&result.acksQueueSize)
	})

	return result
}

func (acker *Acker) SendAck(ack *xgress.Acknowledgement, address xgress.Address) {
	acker.ackIngest <- &ackEntry{
		Acknowledgement: ack,
		Address:         address,
	}
}

func (acker *Acker) ackIngester() {
	var next *ackEntry
	for {
		if next == nil {
			if val, _ := acker.acks.PopFront(); val != nil {
				next = val.(*ackEntry)
			}
		}

		if next == nil {
			select {
			case ack := <-acker.ackIngest:
				acker.acks.PushBack(ack)
			case <-acker.closeNotify:
				return
			}
		} else {
			select {
			case ack := <-acker.ackIngest:
				acker.acks.PushBack(ack)
			case acker.ackSend <- next:
				next = nil
			case <-acker.closeNotify:
				return
			}
		}
		atomic.StoreInt64(&acker.acksQueueSize, int64(acker.acks.Len()))
	}
}

func (acker *Acker) ackSender() {
	logger := pfxlog.Logger()
	for {
		select {
		case nextAck := <-acker.ackSend:
			if err := acker.forwarder.ForwardAcknowledgement(nextAck.Address, nextAck.Acknowledgement); err != nil {
				logger.WithError(err).Debugf("unexpected error while sending ack from %v", nextAck.Address)
				acker.ackFailures.Mark(1)
			} else {
				acker.ackTxMeter.Mark(1)
			}
		case <-acker.closeNotify:
			return
		}
	}
}
