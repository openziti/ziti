/*
	Copyright NetFoundry, Inc.

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
	"fmt"
	"github.com/biogo/store/llrb"
	"github.com/ef-ds/deque"
	"github.com/emirpasic/gods/trees/btree"
	"github.com/emirpasic/gods/utils"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/info"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"math"
	"reflect"
	"strings"
	"sync/atomic"
	"time"
)

type PayloadBufferForwarder interface {
	ForwardPayload(srcAddr Address, payload *Payload) error
	ForwardAcknowledgement(srcAddr Address, acknowledgement *Acknowledgement) error
}

type PayloadBufferController struct {
	forwarder                    PayloadBufferForwarder
	retransmits                  *llrb.Tree
	retransmitIngest             chan *retransmit
	retransmitSend               chan *retransmit
	acks                         *deque.Deque
	ackIngest                    chan *ackEntry
	ackSend                      chan *ackEntry
	ackTicker                    *time.Ticker
	acksQueueSize                int64
	retransmitsQueueSize         int64
	buffersBlockedByLocalWindow  int64
	buffersBlockedByRemoteWindow int64
}

func NewPayloadBufferController(forwarder PayloadBufferForwarder, metrics metrics.Registry) *PayloadBufferController {
	ctrl := &PayloadBufferController{
		forwarder:        forwarder,
		retransmits:      &llrb.Tree{},
		retransmitIngest: make(chan *retransmit, 16),
		retransmitSend:   make(chan *retransmit, 1),
		acks:             deque.New(),
		ackIngest:        make(chan *ackEntry, 16),
		ackSend:          make(chan *ackEntry, 1),
		ackTicker:        time.NewTicker(250 * time.Millisecond),
	}

	go ctrl.ackIngester()
	go ctrl.ackSender()
	go ctrl.retransmitIngester()
	go ctrl.retransmitSender()

	metrics.FuncGauge("xgress.acks.queue_size", func() int64 {
		return atomic.LoadInt64(&ctrl.acksQueueSize)
	})

	metrics.FuncGauge("xgress.retransmits.queue_size", func() int64 {
		return atomic.LoadInt64(&ctrl.retransmitsQueueSize)
	})

	metrics.FuncGauge("xgress.blocked_by_local_window", func() int64 {
		return atomic.LoadInt64(&ctrl.buffersBlockedByLocalWindow)
	})

	metrics.FuncGauge("xgress.blocked_by_remote_window", func() int64 {
		return atomic.LoadInt64(&ctrl.buffersBlockedByRemoteWindow)
	})

	return ctrl
}

func (controller *PayloadBufferController) retransmit(p *payloadAge, address Address) {
	controller.retransmitIngest <- &retransmit{
		payloadAge: p,
		Address:    address,
	}
}

func (controller *PayloadBufferController) ack(ack *Acknowledgement, address Address) {
	controller.ackIngest <- &ackEntry{
		Acknowledgement: ack,
		Address:         address,
	}
}

func (controller *PayloadBufferController) retransmitIngester() {
	var next *retransmit
	for {
		if next == nil && controller.retransmits.Count > 0 {
			next = controller.retransmits.Max().(*retransmit)
			controller.retransmits.DeleteMax()
		}

		if next == nil {
			select {
			case retransmit := <-controller.retransmitIngest:
				controller.retransmits.Insert(retransmit)
			}
		} else {
			select {
			case retransmit := <-controller.retransmitIngest:
				controller.retransmits.Insert(retransmit)
			case controller.retransmitSend <- next:
				next = nil
			}
		}
		atomic.StoreInt64(&controller.retransmitsQueueSize, int64(controller.retransmits.Count))
	}
}

func (controller *PayloadBufferController) ackIngester() {
	var next *ackEntry
	for {
		if next == nil {
			if val, _ := controller.acks.PopFront(); val != nil {
				next = val.(*ackEntry)
			}
		}

		if next == nil {
			select {
			case ack := <-controller.ackIngest:
				controller.acks.PushBack(ack)
			}
		} else {
			select {
			case ack := <-controller.ackIngest:
				controller.acks.PushBack(ack)
			case controller.ackSend <- next:
				next = nil
			}
		}
		atomic.StoreInt64(&controller.acksQueueSize, int64(controller.acks.Len()))
	}
}

func (controller *PayloadBufferController) ackSender() {
	logger := pfxlog.Logger()
	for nextAck := range controller.ackSend {
		now := time.Now()
		if err := controller.forwarder.ForwardAcknowledgement(nextAck.Address, nextAck.Acknowledgement); err != nil {
			logger.WithError(err).Errorf("unexpected error while sending ack from %v", nextAck.Address)
			ackFailures.Mark(1)
		} else {
			ackWriteTimer.UpdateSince(now)
			ackTxMeter.Mark(1)
		}
	}
}

func (controller *PayloadBufferController) retransmitSender() {
	logger := pfxlog.Logger()
	for retransmit := range controller.retransmitSend {
		if err := controller.forwarder.ForwardPayload(retransmit.Address, retransmit.payloadAge.payload); err != nil {
			logger.WithError(err).Errorf("unexpected error while retransmitting payload from %v", retransmit.Address)
			retransmissionFailures.Mark(1)
		} else {
			retransmissions.Mark(1)
			retransmit.markSent()
		}
	}
}

type PayloadBuffer struct {
	x                     *Xgress
	buffer                map[int32]*payloadAge
	newlyBuffered         chan *payloadAge
	acked                 map[int32]int64
	lastAck               int64
	newlyAcknowledged     chan *Payload
	newlyReceivedAcks     chan *Acknowledgement
	receivedAckHwm        int32
	controller            *PayloadBufferController
	transmitBuffer        TransmitBuffer
	windowsSize           uint32
	rxBufferSize          uint32
	txBufferSize          uint32
	mostRecentRTT         uint16
	closeNotify           chan struct{}
	closed                concurrenz.AtomicBoolean
	blockedByLocalWindow  bool
	blockedByRemoteWindow bool
	lastBufferSizeSent    uint32

	config struct {
		retransmitAge uint16
		ackPeriod     int64
		ackCount      int
		idleAckAfter  int64
	}
}

type payloadAge struct {
	payload *Payload
	age     int64
}

func (self *payloadAge) markSent() {
	atomic.StoreInt64(&self.age, info.NowInMilliseconds())
}

func (self *payloadAge) getAge() int64 {
	return atomic.LoadInt64(&self.age)
}

func NewPayloadBuffer(x *Xgress, controller *PayloadBufferController) *PayloadBuffer {
	buffer := &PayloadBuffer{
		x:                 x,
		buffer:            make(map[int32]*payloadAge),
		newlyBuffered:     make(chan *payloadAge, x.Options.TxQueueSize),
		acked:             make(map[int32]int64),
		lastAck:           info.NowInMilliseconds(),
		newlyAcknowledged: make(chan *Payload),
		newlyReceivedAcks: make(chan *Acknowledgement),
		receivedAckHwm:    -1,
		controller:        controller,
		transmitBuffer: TransmitBuffer{
			tree:     btree.NewWith(10240, utils.Int32Comparator),
			sequence: -1,
		},
		windowsSize: 256 * 1024,
		closeNotify: make(chan struct{}),
	}

	buffer.config.retransmitAge = 2000
	buffer.config.ackPeriod = 64
	buffer.config.ackCount = 96
	buffer.config.idleAckAfter = 5000

	go buffer.run()
	return buffer
}

func (buffer *PayloadBuffer) BufferPayload(payload *Payload) (func(), error) {
	if buffer.x.sessionId != payload.GetSessionId() {
		return nil, errors.Errorf("bad payload. should have session %v, but had %v", buffer.x.sessionId, payload.GetSessionId())
	}

	payloadAge := &payloadAge{payload: payload, age: math.MaxInt64}
	select {
	case buffer.newlyBuffered <- payloadAge:
		pfxlog.ContextLogger("s/"+payload.GetSessionId()).Debugf("buffered [%d]", payload.GetSequence())
		return payloadAge.markSent, nil
	case <-buffer.closeNotify:
		return nil, errors.Errorf("payload buffer closed")
	}
}

func (buffer *PayloadBuffer) PayloadReceived(payload *Payload) {
	if buffer.transmitBuffer.Size() < 256*1024 || payload.Sequence < buffer.transmitBuffer.sequence+11 {
		log := pfxlog.Logger().WithFields(payload.GetLoggerFields())
		log.Debug("payload received")
		buffer.transmitBuffer.ReceiveUnordered(payload)
		select {
		case buffer.newlyAcknowledged <- payload:
			log.Debug("payload ackknowledged")
		case <-buffer.closeNotify:
			log.Error("payload buffer closed")
		}
	} else {
		droppedPayloadsMeter.Mark(1)
	}
}

func (buffer *PayloadBuffer) ReceiveAcknowledgement(ack *Acknowledgement) {
	log := pfxlog.Logger().WithFields(ack.GetLoggerFields())
	log.Debug("ack received")
	select {
	case buffer.newlyReceivedAcks <- ack:
		log.Debug("ack processed")
	case <-buffer.closeNotify:
		log.Error("payload buffer closed")
	}
}

func (buffer *PayloadBuffer) Close() {
	logrus.Debugf("[%p] closing", buffer)
	if buffer.closed.CompareAndSwap(false, true) {
		close(buffer.closeNotify)
	}
}

func (buffer *PayloadBuffer) isBlocked() bool {
	blocked := false

	if buffer.windowsSize < buffer.txBufferSize {
		blocked = true
		if !buffer.blockedByRemoteWindow {
			buffer.blockedByRemoteWindow = true
			atomic.AddInt64(&buffer.controller.buffersBlockedByRemoteWindow, 1)
		}
	} else if buffer.blockedByRemoteWindow {
		buffer.blockedByRemoteWindow = false
		atomic.AddInt64(&buffer.controller.buffersBlockedByRemoteWindow, -1)
	}

	if buffer.windowsSize < buffer.rxBufferSize {
		blocked = true
		if !buffer.blockedByLocalWindow {
			buffer.blockedByLocalWindow = true
			atomic.AddInt64(&buffer.controller.buffersBlockedByLocalWindow, 1)
		}
	} else if buffer.blockedByLocalWindow {
		buffer.blockedByLocalWindow = false
		atomic.AddInt64(&buffer.controller.buffersBlockedByLocalWindow, -1)
	}

	pfxlog.Logger().Tracef("blocked=%v tx_buffer_size=%v rx_buffer_size=%v", blocked, buffer.txBufferSize, buffer.rxBufferSize)

	return blocked
}

func (buffer *PayloadBuffer) run() {
	log := pfxlog.ContextLogger("s/" + buffer.x.sessionId)
	defer log.Debugf("[%p] exited", buffer)
	log.Debugf("[%p] started", buffer)

	lastDebug := info.NowInMilliseconds()

	var buffered chan *payloadAge

	for {
		rxBufferSizeHistogram.Update(int64(buffer.rxBufferSize))

		if buffer.isBlocked() {
			buffered = nil
		} else {
			buffered = buffer.newlyBuffered
		}

		now := info.NowInMilliseconds()
		if now-lastDebug >= 2000 {
			buffer.debug(now)
			lastDebug = now
		}

		select {
		case ack := <-buffer.newlyReceivedAcks:
			if err := buffer.receiveAcknowledgement(ack); err != nil {
				log.Errorf("unexpected error (%s)", err)
			}
			if err := buffer.retransmit(); err != nil {
				log.Errorf("unexpected error retransmitting (%s)", err)
			}

		case payload := <-buffer.newlyAcknowledged:
			if err := buffer.acknowledgePayload(payload); err != nil {
				log.Errorf("unexpected error (%s)", err)
			} else if payload.RTT != 0 {
				buffer.mostRecentRTT = uint16(info.NowInMilliseconds()) - payload.RTT
			}
			//if err := buffer.acknowledge(); err != nil {
			//	log.Errorf("unexpected error (%s)", err)
			//}

		case payloadAge := <-buffered:
			buffer.buffer[payloadAge.payload.GetSequence()] = payloadAge
			buffer.rxBufferSize += uint32(len(payloadAge.payload.Data))
			log.Tracef("buffering payload %v with size %v. payload buffer size: %v",
				payloadAge.payload.Sequence, len(payloadAge.payload.Data), buffer.rxBufferSize)
			//if err := buffer.acknowledge(); err != nil {
			//	log.Errorf("unexpected error (%s)", err)
			//}

		case <-time.After(time.Duration(buffer.config.idleAckAfter) * time.Millisecond):
			//if err := buffer.acknowledge(); err != nil {
			//	log.Errorf("unexpected error acknowledging (%s)", err)
			//}
			if err := buffer.retransmit(); err != nil {
				log.Errorf("unexpected error retransmitting (%s)", err)
			}
		case <-buffer.closeNotify:
			if buffer.blockedByLocalWindow {
				atomic.AddInt64(&buffer.controller.buffersBlockedByLocalWindow, -1)
			}
			if buffer.blockedByRemoteWindow {
				atomic.AddInt64(&buffer.controller.buffersBlockedByRemoteWindow, -1)
			}
			return
		}
	}
}

func (buffer *PayloadBuffer) acknowledgePayload(payload *Payload) error {
	if buffer.x.sessionId != payload.SessionId {
		return errors.New("unexpected Payload")
	}

	log := pfxlog.Logger().WithFields(payload.GetLoggerFields())
	log.Debug("ready to acknowledge")

	ack := NewAcknowledgement(buffer.x.sessionId, buffer.x.originator)
	ack.TxBufferSize = buffer.transmitBuffer.Size()
	ack.Sequence = append(ack.Sequence, payload.Sequence)
	if buffer.mostRecentRTT != 0 {
		ack.RTT = buffer.mostRecentRTT
		buffer.mostRecentRTT = 0
	}
	log.Debugf("acknowledging 1 payload, [%d] buffered", len(buffer.buffer))

	atomic.StoreUint32(&buffer.lastBufferSizeSent, ack.TxBufferSize)
	buffer.controller.ack(ack, buffer.x.address)

	return nil
}

func (buffer *PayloadBuffer) SendEmptyAck() {
	pfxlog.Logger().WithField("session", buffer.x.sessionId).Debug("sending empty ack")
	ack := NewAcknowledgement(buffer.x.sessionId, buffer.x.originator)
	ack.TxBufferSize = buffer.transmitBuffer.Size()
	atomic.StoreUint32(&buffer.lastBufferSizeSent, ack.TxBufferSize)
	buffer.controller.ack(ack, buffer.x.address)
}

func (buffer *PayloadBuffer) getLastBufferSizeSent() uint32 {
	return atomic.LoadUint32(&buffer.lastBufferSizeSent)
}

//func (buffer *PayloadBuffer) acknowledgePayload(payload *Payload) error {
//	if buffer.x.sessionId == payload.SessionId {
//		buffer.acked[payload.Sequence] = info.NowInMilliseconds()
//	} else {
//		return errors.New("unexpected Payload")
//	}
//
//	return nil
//}

func (buffer *PayloadBuffer) receiveAcknowledgement(ack *Acknowledgement) error {
	log := pfxlog.Logger().WithFields(ack.GetLoggerFields())
	if buffer.x.sessionId == ack.SessionId {
		for _, sequence := range ack.Sequence {
			if sequence > buffer.receivedAckHwm {
				buffer.receivedAckHwm = sequence
			}
			if payloadAge, found := buffer.buffer[sequence]; found {
				delete(buffer.buffer, sequence)
				buffer.rxBufferSize -= uint32(len(payloadAge.payload.Data))
				log.Debugf("removing payload %v with size %v. payload buffer size: %v",
					payloadAge.payload.Sequence, len(payloadAge.payload.Data), buffer.rxBufferSize)
			}
		}
		buffer.txBufferSize = ack.TxBufferSize
		remoteTxBufferSizeHistogram.Update(int64(buffer.txBufferSize))
		if ack.RTT > 0 {
			buffer.config.retransmitAge = uint16(float32(ack.RTT) * 1.2)
			rttHistogram.Update(int64(ack.RTT))
		}
	} else {
		return errors.New("unexpected acknowledgement")
	}
	return nil
}

func (buffer *PayloadBuffer) acknowledge() error {
	log := pfxlog.ContextLogger("s/" + buffer.x.sessionId)
	now := info.NowInMilliseconds()

	if now-buffer.lastAck >= buffer.config.ackPeriod || len(buffer.acked) >= buffer.config.ackCount {
		log.Debug("ready to acknowledge")

		ack := NewAcknowledgement(buffer.x.sessionId, buffer.x.originator)
		ack.TxBufferSize = buffer.transmitBuffer.Size()
		for sequence := range buffer.acked {
			ack.Sequence = append(ack.Sequence, sequence)
		}
		if buffer.mostRecentRTT != 0 {
			ack.RTT = buffer.mostRecentRTT
			buffer.mostRecentRTT = 0
		}
		log.Debugf("acknowledging [%d] payloads, [%d] buffered", len(ack.Sequence), len(buffer.buffer))

		buffer.controller.ack(ack, buffer.x.address)

		buffer.acked = make(map[int32]int64) // clear
		buffer.lastAck = now
	} else {
		log.Debug("not ready to acknowledge")
	}

	return nil
}

func (buffer *PayloadBuffer) retransmit() error {
	if len(buffer.buffer) > 0 {
		log := pfxlog.ContextLogger(fmt.Sprintf("s/" + buffer.x.sessionId))

		now := info.NowInMilliseconds()
		retransmitted := 0
		for _, v := range buffer.buffer {
			if v.payload.GetSequence() < buffer.receivedAckHwm && uint16(now-v.getAge()) > buffer.config.retransmitAge {
				buffer.controller.retransmit(v, buffer.x.address)
				retransmitted++
			}
		}

		if retransmitted > 0 {
			log.Infof("retransmitted [%d] payloads, [%d] buffered, rxBufferSize", retransmitted, len(buffer.buffer), buffer.rxBufferSize)
		}
	}
	return nil
}

func (buffer *PayloadBuffer) debug(now int64) {
	pfxlog.ContextLogger(buffer.x.sessionId).Debugf("buffer=[%d], acked=[%d], lastAck=[%d ms.], receivedAckHwm=[%d]",
		len(buffer.buffer), len(buffer.acked), now-buffer.lastAck, buffer.receivedAckHwm)
}

type retransmit struct {
	*payloadAge
	session string
	Address
}

func (r *retransmit) Compare(comparable llrb.Comparable) int {
	if other, ok := comparable.(*retransmit); ok {
		if result := int(r.age - other.age); result != 0 {
			return result
		}
		if result := int(r.payload.Sequence - other.payload.Sequence); result != 0 {
			return result
		}
		return strings.Compare(r.session, other.session)
	}
	pfxlog.Logger().Errorf("*retransmit was compared to %v, which should not happen", reflect.TypeOf(comparable))
	return -1
}

type ackEntry struct {
	Address
	*Acknowledgement
}
