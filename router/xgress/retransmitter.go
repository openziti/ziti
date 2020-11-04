package xgress

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/metrics"
	"sync/atomic"
)

var retransmitter *Retransmitter

func InitRetransmitter(forwarder PayloadBufferForwarder, metrics metrics.Registry) {
	retransmitter = NewRetransmitter(forwarder, metrics)
}

type Retransmitter struct {
	forwarder            PayloadBufferForwarder
	retxTail             *txPayload
	retxHead             *txPayload
	retransmitIngest     chan *txPayload
	retransmitSend       chan *txPayload
	retransmitsQueueSize int64
}

func NewRetransmitter(forwarder PayloadBufferForwarder, metrics metrics.Registry) *Retransmitter {
	ctrl := &Retransmitter{
		forwarder:        forwarder,
		retransmitIngest: make(chan *txPayload, 16),
		retransmitSend:   make(chan *txPayload, 1),
	}

	go ctrl.retransmitIngester()
	go ctrl.retransmitSender()

	metrics.FuncGauge("xgress.retransmits.queue_size", func() int64 {
		return atomic.LoadInt64(&ctrl.retransmitsQueueSize)
	})

	return ctrl
}

func (retransmitter *Retransmitter) queue(p *txPayload) {
	retransmitter.retransmitIngest <- p
}

func (retransmitter *Retransmitter) popHead() *txPayload {
	if retransmitter.retxHead == nil {
		return nil
	}

	result := retransmitter.retxHead
	if result.prev == nil {
		retransmitter.retxHead = nil
		retransmitter.retxTail = nil
	} else {
		retransmitter.retxHead = result.prev
		result.prev.next = nil
	}

	result.prev = nil
	result.next = nil

	atomic.AddInt64(&retransmitter.retransmitsQueueSize, -1)

	return result
}

func (retransmitter *Retransmitter) pushTail(txp *txPayload) {
	if txp.prev != nil || txp.next != nil || txp == retransmitter.retxHead {
		return
	}
	if retransmitter.retxHead == nil {
		retransmitter.retxTail = txp
		retransmitter.retxHead = txp
	} else {
		txp.next = retransmitter.retxTail
		retransmitter.retxTail.prev = txp
		retransmitter.retxTail = txp
	}
	atomic.AddInt64(&retransmitter.retransmitsQueueSize, 1)
}

func (retransmitter *Retransmitter) delete(txp *txPayload) {
	if retransmitter.retxHead == txp {
		retransmitter.popHead()
	} else if txp == retransmitter.retxTail {
		retransmitter.retxTail = txp.next
		retransmitter.retxTail.prev = nil
		atomic.AddInt64(&retransmitter.retransmitsQueueSize, -1)
	} else if txp.prev != nil {
		txp.prev.next = txp.next
		txp.next.prev = txp.prev
		atomic.AddInt64(&retransmitter.retransmitsQueueSize, -1)
	}

	txp.prev = nil
	txp.next = nil
}

func (retransmitter *Retransmitter) retransmitIngester() {
	var next *txPayload
	for {
		if next == nil {
			next = retransmitter.popHead()
		}

		if next == nil {
			select {
			case retransmit := <-retransmitter.retransmitIngest:
				retransmitter.acceptRetransmit(retransmit)
			}
		} else {
			select {
			case retransmit := <-retransmitter.retransmitIngest:
				retransmitter.acceptRetransmit(retransmit)
			case retransmitter.retransmitSend <- next:
				next = nil
			}
		}
	}
}

func (retransmitter *Retransmitter) acceptRetransmit(txp *txPayload) {
	if txp.isAcked() {
		retransmitter.delete(txp)
	} else {
		retransmitter.pushTail(txp)
	}
}

func (retransmitter *Retransmitter) retransmitSender() {
	logger := pfxlog.Logger()
	for retransmit := range retransmitter.retransmitSend {
		if !retransmit.isAcked() {
			if err := retransmitter.forwarder.ForwardPayload(retransmit.x.address, retransmit.payload); err != nil {
				logger.WithError(err).Errorf("unexpected error while retransmitting payload from %v", retransmit.x.address)
				retransmissionFailures.Mark(1)
			} else {
				retransmit.markSent()
				retransmissions.Mark(1)
			}
			retransmit.dequeued()
		}
	}
}
