package xgress

import (
	"github.com/ef-ds/deque"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/metrics"
	"sync/atomic"
)

var acker *Acker

func InitAcker(forwarder PayloadBufferForwarder, metrics metrics.Registry, closeNotify <-chan struct{}) {
	acker = NewAcker(forwarder, metrics, closeNotify)
}

type ackEntry struct {
	Address
	*Acknowledgement
}

type Acker struct {
	forwarder     PayloadBufferForwarder
	acks          *deque.Deque
	ackIngest     chan *ackEntry
	ackSend       chan *ackEntry
	acksQueueSize int64
	closeNotify   <-chan struct{}
}

func NewAcker(forwarder PayloadBufferForwarder, metrics metrics.Registry, closeNotify <-chan struct{}) *Acker {
	result := &Acker{
		forwarder:   forwarder,
		acks:        deque.New(),
		ackIngest:   make(chan *ackEntry, 16),
		ackSend:     make(chan *ackEntry, 1),
		closeNotify: closeNotify,
	}

	go result.ackIngester()
	go result.ackSender()

	metrics.FuncGauge("xgress.acks.queue_size", func() int64 {
		return atomic.LoadInt64(&result.acksQueueSize)
	})

	return result
}

func (acker *Acker) ack(ack *Acknowledgement, address Address) {
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
				ackFailures.Mark(1)
			} else {
				ackTxMeter.Mark(1)
			}
		case <-acker.closeNotify:
			return
		}
	}
}
