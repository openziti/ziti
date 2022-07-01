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

package xgress

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/inspect"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/info"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"math"
	"sync/atomic"
	"time"
)

type PayloadBufferForwarder interface {
	ForwardPayload(srcAddr Address, payload *Payload) error
	ForwardAcknowledgement(srcAddr Address, acknowledgement *Acknowledgement) error
}

type LinkSendBuffer struct {
	x                     *Xgress
	buffer                map[int32]*txPayload
	newlyBuffered         chan *txPayload
	newlyReceivedAcks     chan *Acknowledgement
	windowsSize           uint32
	linkSendBufferSize    uint32
	linkRecvBufferSize    uint32
	accumulator           uint32
	successfulAcks        uint32
	duplicateAcks         uint32
	retransmits           uint32
	closeNotify           chan struct{}
	closed                concurrenz.AtomicBoolean
	blockedByLocalWindow  bool
	blockedByRemoteWindow bool
	retxScale             float64
	retxThreshold         uint32
	lastRtt               uint16
	lastRetransmitTime    int64
	closeWhenEmpty        concurrenz.AtomicBoolean
	inspectRequests       chan *sendBufferInspectEvent
}

type txPayload struct {
	payload    *Payload
	age        int64
	retxQueued int32
	x          *Xgress
	next       *txPayload
	prev       *txPayload
}

func (self *txPayload) markSent() {
	atomic.StoreInt64(&self.age, info.NowInMilliseconds())
}

func (self *txPayload) getAge() int64 {
	return atomic.LoadInt64(&self.age)
}

func (self *txPayload) markQueued() {
	atomic.AddInt32(&self.retxQueued, 1)
}

// markAcked marks the payload and acked and returns true if the payload is queued for retransmission
func (self *txPayload) markAcked() bool {
	return atomic.AddInt32(&self.retxQueued, 2) > 2
}

func (self *txPayload) dequeued() {
	atomic.AddInt32(&self.retxQueued, -1)
}

func (self *txPayload) isAcked() bool {
	return atomic.LoadInt32(&self.retxQueued) > 1
}

func (self *txPayload) isRetransmittable() bool {
	return atomic.LoadInt32(&self.retxQueued) == 0
}

func NewLinkSendBuffer(x *Xgress) *LinkSendBuffer {
	logrus.Debugf("txPortalStartSize = %d", x.Options.TxPortalStartSize)

	buffer := &LinkSendBuffer{
		x:                 x,
		buffer:            make(map[int32]*txPayload),
		newlyBuffered:     make(chan *txPayload, x.Options.TxQueueSize),
		newlyReceivedAcks: make(chan *Acknowledgement),
		closeNotify:       make(chan struct{}),
		windowsSize:       x.Options.TxPortalStartSize,
		retxThreshold:     x.Options.RetxStartMs,
		retxScale:         x.Options.RetxScale,
		inspectRequests:   make(chan *sendBufferInspectEvent, 1),
	}

	go buffer.run()
	return buffer
}

func (buffer *LinkSendBuffer) CloseWhenEmpty() bool {
	return buffer.closeWhenEmpty.CompareAndSwap(false, true)
}

func (buffer *LinkSendBuffer) BufferPayload(payload *Payload) (func(), error) {
	txPayload := &txPayload{payload: payload, age: math.MaxInt64, x: buffer.x}
	select {
	case buffer.newlyBuffered <- txPayload:
		pfxlog.ContextLogger(buffer.x.Label()).Debugf("buffered [%d]", payload.GetSequence())
		return txPayload.markSent, nil
	case <-buffer.closeNotify:
		return nil, errors.Errorf("payload buffer closed")
	}
}

func (buffer *LinkSendBuffer) ReceiveAcknowledgement(ack *Acknowledgement) {
	log := pfxlog.ContextLogger(buffer.x.Label()).WithFields(ack.GetLoggerFields())
	log.Debug("ack received")
	select {
	case buffer.newlyReceivedAcks <- ack:
		log.Debug("ack processed")
	case <-buffer.closeNotify:
		log.Error("payload buffer closed")
	}
}

func (buffer *LinkSendBuffer) Close() {
	pfxlog.ContextLogger(buffer.x.Label()).Debugf("[%p] closing", buffer)
	if buffer.closed.CompareAndSwap(false, true) {
		close(buffer.closeNotify)
	}
}

func (buffer *LinkSendBuffer) isBlocked() bool {
	blocked := false

	if buffer.windowsSize < buffer.linkRecvBufferSize {
		blocked = true
		if !buffer.blockedByRemoteWindow {
			buffer.blockedByRemoteWindow = true
			atomic.AddInt64(&buffersBlockedByRemoteWindow, 1)
		}
	} else if buffer.blockedByRemoteWindow {
		buffer.blockedByRemoteWindow = false
		atomic.AddInt64(&buffersBlockedByRemoteWindow, -1)
	}

	if buffer.windowsSize < buffer.linkSendBufferSize {
		blocked = true
		if !buffer.blockedByLocalWindow {
			buffer.blockedByLocalWindow = true
			atomic.AddInt64(&buffersBlockedByLocalWindow, 1)
		}
	} else if buffer.blockedByLocalWindow {
		buffer.blockedByLocalWindow = false
		atomic.AddInt64(&buffersBlockedByLocalWindow, -1)
	}

	if blocked {
		pfxlog.ContextLogger(buffer.x.Label()).Debugf("blocked=%v win_size=%v tx_buffer_size=%v rx_buffer_size=%v", blocked, buffer.windowsSize, buffer.linkRecvBufferSize, buffer.linkSendBufferSize)
	}

	return blocked
}

func (buffer *LinkSendBuffer) run() {
	log := pfxlog.ContextLogger(buffer.x.Label())
	defer log.Debugf("[%p] exited", buffer)
	log.Debugf("[%p] started", buffer)

	var buffered chan *txPayload

	retransmitTicker := time.NewTicker(100 * time.Millisecond)
	defer retransmitTicker.Stop()

	for {
		// don't block when we're closing, since the only thing that should still be coming in is end-of-circuit
		// if we're blocked, but empty, let one payload in to reduce the chances of a stall
		if buffer.isBlocked() && !buffer.closeWhenEmpty.Get() && buffer.linkSendBufferSize != 0 {
			buffered = nil
		} else {
			buffered = buffer.newlyBuffered
		}

		// bias acks by allowing 10 acks to be processed for every payload in
		for i := 0; i < 10; i++ {
			select {
			case ack := <-buffer.newlyReceivedAcks:
				buffer.receiveAcknowledgement(ack)
			case <-buffer.closeNotify:
				buffer.close()
				return
			default:
				i = 10
			}
		}

		select {
		case inspectEvent := <-buffer.inspectRequests:
			inspectEvent.handle(buffer)

		case ack := <-buffer.newlyReceivedAcks:
			buffer.receiveAcknowledgement(ack)
			buffer.retransmit()
			if buffer.closeWhenEmpty.Get() && len(buffer.buffer) == 0 && !buffer.x.Closed() && buffer.x.IsEndOfCircuitSent() {
				go buffer.x.Close()
			}

		case txPayload := <-buffered:
			buffer.buffer[txPayload.payload.GetSequence()] = txPayload
			payloadSize := len(txPayload.payload.Data)
			buffer.linkSendBufferSize += uint32(payloadSize)
			atomic.AddInt64(&outstandingPayloads, 1)
			atomic.AddInt64(&outstandingPayloadBytes, int64(payloadSize))
			log.Tracef("buffering payload %v with size %v. payload buffer size: %v",
				txPayload.payload.Sequence, len(txPayload.payload.Data), buffer.linkSendBufferSize)

		case <-retransmitTicker.C:
			buffer.retransmit()

		case <-buffer.closeNotify:
			buffer.close()
			return
		}
	}
}

func (buffer *LinkSendBuffer) close() {
	if buffer.blockedByLocalWindow {
		atomic.AddInt64(&buffersBlockedByLocalWindow, -1)
	}
	if buffer.blockedByRemoteWindow {
		atomic.AddInt64(&buffersBlockedByRemoteWindow, -1)
	}
}

func (buffer *LinkSendBuffer) receiveAcknowledgement(ack *Acknowledgement) {
	log := pfxlog.ContextLogger(buffer.x.Label()).WithFields(ack.GetLoggerFields())

	for _, sequence := range ack.Sequence {
		if txPayload, found := buffer.buffer[sequence]; found {
			if txPayload.markAcked() { // if it's been queued for retransmission, remove it from the queue
				retransmitter.queue(txPayload)
			}

			payloadSize := uint32(len(txPayload.payload.Data))
			buffer.accumulator += payloadSize
			buffer.successfulAcks++
			delete(buffer.buffer, sequence)
			atomic.AddInt64(&outstandingPayloads, -1)
			atomic.AddInt64(&outstandingPayloadBytes, -int64(payloadSize))
			buffer.linkSendBufferSize -= payloadSize
			log.Debugf("removing payload %v with size %v. payload buffer size: %v",
				txPayload.payload.Sequence, len(txPayload.payload.Data), buffer.linkSendBufferSize)

			if buffer.successfulAcks >= buffer.x.Options.TxPortalIncreaseThresh {
				buffer.successfulAcks = 0
				delta := uint32(float64(buffer.accumulator) * buffer.x.Options.TxPortalIncreaseScale)
				buffer.windowsSize += delta
				if buffer.windowsSize > buffer.x.Options.TxPortalMaxSize {
					buffer.windowsSize = buffer.x.Options.TxPortalMaxSize
				}
				buffer.retxScale -= 0.02
				if buffer.retxScale < buffer.x.Options.RetxScale {
					buffer.retxScale = buffer.x.Options.RetxScale
				}
			}
		} else { // duplicate ack
			duplicateAcksMeter.Mark(1)
			buffer.duplicateAcks++
			if buffer.duplicateAcks >= buffer.x.Options.TxPortalDupAckThresh {
				buffer.duplicateAcks = 0
				buffer.retxScale += 0.2
			}
		}
	}

	buffer.linkRecvBufferSize = ack.RecvBufferSize
	if ack.RTT > 0 {
		rtt := uint16(info.NowInMilliseconds()) - ack.RTT
		if buffer.lastRtt > 0 {
			rtt = (rtt + buffer.lastRtt) >> 1
		}
		buffer.lastRtt = rtt
		buffer.retxThreshold = uint32(float64(rtt)*buffer.retxScale) + buffer.x.Options.RetxAddMs
	}
}

func (buffer *LinkSendBuffer) retransmit() {
	now := info.NowInMilliseconds()
	if len(buffer.buffer) > 0 && (now-buffer.lastRetransmitTime) > 64 {
		log := pfxlog.ContextLogger(buffer.x.Label())

		retransmitted := 0
		for _, v := range buffer.buffer {
			if v.isRetransmittable() && uint32(now-v.getAge()) >= buffer.retxThreshold {
				v.markQueued()
				retransmitter.queue(v)
				retransmitted++
				buffer.retransmits++
				if buffer.retransmits >= buffer.x.Options.TxPortalRetxThresh {
					buffer.accumulator = 0
					buffer.retransmits = 0
					buffer.scale(buffer.x.Options.TxPortalRetxScale)
				}
			}
		}

		if retransmitted > 0 {
			log.Debugf("retransmitted [%d] payloads, [%d] buffered, linkSendBufferSize: %d", retransmitted, len(buffer.buffer), buffer.linkSendBufferSize)
		}
		buffer.lastRetransmitTime = now
	}
}

func (buffer *LinkSendBuffer) scale(factor float64) {
	buffer.windowsSize = uint32(float64(buffer.windowsSize) * factor)
	if factor > 1 {
		if buffer.windowsSize > buffer.x.Options.TxPortalMaxSize {
			buffer.windowsSize = buffer.x.Options.TxPortalMaxSize
		}
	} else if buffer.windowsSize < buffer.x.Options.TxPortalMinSize {
		buffer.windowsSize = buffer.x.Options.TxPortalMinSize
	}
}

func (buffer *LinkSendBuffer) inspect() *inspect.XgressSendBufferDetail {
	timeSinceLastRetransmit := time.Duration(info.NowInMilliseconds()-buffer.lastRetransmitTime) * time.Millisecond
	result := &inspect.XgressSendBufferDetail{
		WindowSize:            buffer.windowsSize,
		LinkSendBufferSize:    buffer.linkSendBufferSize,
		LinkRecvBufferSize:    buffer.linkRecvBufferSize,
		Accumulator:           buffer.accumulator,
		SuccessfulAcks:        buffer.successfulAcks,
		DuplicateAcks:         buffer.duplicateAcks,
		Retransmits:           buffer.retransmits,
		Closed:                buffer.closed.Get(),
		BlockedByLocalWindow:  buffer.blockedByLocalWindow,
		BlockedByRemoteWindow: buffer.blockedByRemoteWindow,
		RetxScale:             buffer.retxScale,
		RetxThreshold:         buffer.retxThreshold,
		TimeSinceLastRetx:     timeSinceLastRetransmit.String(),
		CloseWhenEmpty:        buffer.closeWhenEmpty.Get(),
	}
	return result
}

func (buffer *LinkSendBuffer) Inspect() *inspect.XgressSendBufferDetail {
	timeout := time.After(100 * time.Millisecond)
	inspectEvent := &sendBufferInspectEvent{
		notifyComplete: make(chan *inspect.XgressSendBufferDetail, 1),
	}

	select {
	case buffer.inspectRequests <- inspectEvent:
		select {
		case result := <-inspectEvent.notifyComplete:
			result.AcquiredSafely = true
			return result
		case <-timeout:
		}
	case <-timeout:
	}

	result := buffer.inspect()
	result.AcquiredSafely = false
	return result
}

type sendBufferInspectEvent struct {
	notifyComplete chan *inspect.XgressSendBufferDetail
}

func (self *sendBufferInspectEvent) handle(buffer *LinkSendBuffer) {
	result := buffer.inspect()
	self.notifyComplete <- result
}
