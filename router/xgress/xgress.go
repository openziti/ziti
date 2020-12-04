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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/info"
	"github.com/openziti/foundation/util/mathz"
	"github.com/sirupsen/logrus"
	"io"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

const (
	HeaderKeyUUID = 0
)

type Address string

type Listener interface {
	Listen(address string, bindHandler BindHandler) error
	Close() error
}

type Dialer interface {
	Dial(destination string, sessionId *identity.TokenId, address Address, bindHandler BindHandler) (xt.PeerData, error)
	IsTerminatorValid(id string, destination string) bool
}

type Factory interface {
	CreateListener(optionsData OptionsData) (Listener, error)
	CreateDialer(optionsData OptionsData) (Dialer, error)
}

type OptionsData map[interface{}]interface{}

// The BindHandlers are invoked to install the appropriate handlers.
//
type BindHandler interface {
	HandleXgressBind(x *Xgress)
}

// ReceiveHandler is invoked by an xgress whenever data is received from the connected peer. Generally a ReceiveHandler
// is implemented to connect the xgress to a data plane data transmission system.
//
type ReceiveHandler interface {
	// HandleXgressReceive is invoked when data is received from the connected xgress peer.
	//
	HandleXgressReceive(payload *Payload, x *Xgress)
}

// CloseHandler is invoked by an xgress when the connected peer terminates the communication.
//
type CloseHandler interface {
	// HandleXgressClose is invoked when the connected peer terminates the communication.
	//
	HandleXgressClose(x *Xgress)
}

// PeekHandler allows registering watcher to react to data flowing an xgress instance
type PeekHandler interface {
	Rx(x *Xgress, payload *Payload)
	Tx(x *Xgress, payload *Payload)
	Close(x *Xgress)
}

type Connection interface {
	io.Closer
	LogContext() string
	ReadPayload() ([]byte, map[uint8][]byte, error)
	WritePayload([]byte, map[uint8][]byte) (int, error)
}

type Xgress struct {
	sessionId      string
	address        Address
	peer           Connection
	originator     Originator
	Options        *Options
	txQueue        chan *Payload
	closeNotify    chan struct{}
	rxSequence     int32
	rxSequenceLock sync.Mutex
	receiveHandler ReceiveHandler
	payloadBuffer  *LinkSendBuffer
	linkRxBuffer   *LinkReceiveBuffer
	closed         concurrenz.AtomicBoolean
	closeHandler   CloseHandler
	peekHandlers   []PeekHandler
	rxerStarted    concurrenz.AtomicBoolean
}

func NewXgress(sessionId *identity.TokenId, address Address, peer Connection, originator Originator, options *Options) *Xgress {
	result := &Xgress{
		sessionId:    sessionId.Token,
		address:      address,
		peer:         peer,
		originator:   originator,
		Options:      options,
		txQueue:      make(chan *Payload, options.TxQueueSize),
		closeNotify:  make(chan struct{}),
		rxSequence:   0,
		linkRxBuffer: NewLinkReceiveBuffer(),
	}
	result.payloadBuffer = NewLinkSendBuffer(result)
	return result
}

func (self *Xgress) SessionId() string {
	return self.sessionId
}

func (self *Xgress) Address() Address {
	return self.address
}

func (self *Xgress) Originator() Originator {
	return self.originator
}

func (self *Xgress) IsTerminator() bool {
	return self.originator == Terminator
}

func (self *Xgress) SetReceiveHandler(receiveHandler ReceiveHandler) {
	self.receiveHandler = receiveHandler
}

func (self *Xgress) SetCloseHandler(closeHandler CloseHandler) {
	self.closeHandler = closeHandler
}

func (self *Xgress) AddPeekHandler(peekHandler PeekHandler) {
	self.peekHandlers = append(self.peekHandlers, peekHandler)
}

func (self *Xgress) Start() {
	log := pfxlog.ContextLogger(self.Label())
	if !self.IsTerminator() {
		log.Debug("initiator: sending session start")
		self.forwardPayload(self.GetStartSession())
		go self.rx()
	} else {
		log.Debug("terminator: waiting for session start before starting receiver")
	}
	go self.tx()
}

func (self *Xgress) Label() string {
	return fmt.Sprintf("{s/%s|@/%s}<%s>", self.sessionId, string(self.address), self.originator.String())
}

func (self *Xgress) GetStartSession() *Payload {
	startSession := &Payload{
		Header: Header{
			SessionId: self.sessionId,
			Flags:     SetOriginatorFlag(uint32(PayloadFlagSessionStart), self.originator),
		},
		Sequence: self.nextReceiveSequence(),
		Data:     nil,
	}
	return startSession
}

func (self *Xgress) GetEndSession() *Payload {
	endSession := &Payload{
		Header: Header{
			SessionId: self.sessionId,
			Flags:     SetOriginatorFlag(uint32(PayloadFlagSessionEnd), self.originator),
		},
		Sequence: self.nextReceiveSequence(),
		Data:     nil,
	}
	return endSession
}

func (self *Xgress) CloseTimeout(duration time.Duration) {
	go self.closeTimeoutHandler(duration)
}

func (self *Xgress) Close() {
	log := pfxlog.ContextLogger(self.Label())
	log.Debug("closing xgress peer")
	if err := self.peer.Close(); err != nil {
		log.WithError(err).Warn("error while closing xgress peer")
	}

	if self.closed.CompareAndSwap(false, true) {
		log.Debug("closing tx queue")
		close(self.closeNotify)

		self.payloadBuffer.Close()

		for _, peekHandler := range self.peekHandlers {
			peekHandler.Close(self)
		}

		if self.closeHandler != nil {
			self.closeHandler.HandleXgressClose(self)
		} else {
			pfxlog.ContextLogger(self.Label()).Warn("no close handler")
		}
	} else {
		log.Debug("xgress already closed, skipping close")
	}
}

func (self *Xgress) Closed() bool {
	return self.closed.Get()
}

func (self *Xgress) SendPayload(payload *Payload) error {
	if self.closed.Get() {
		return nil
	}

	if payload.IsSessionEndFlagSet() {
		pfxlog.ContextLogger(self.Label()).Debug("received end of session Payload")
	}

	payloadIngester.ingest(payload, self)

	return nil
}

func (self *Xgress) SendAcknowledgement(acknowledgement *Acknowledgement) error {
	ackRxMeter.Mark(1)
	self.payloadBuffer.ReceiveAcknowledgement(acknowledgement)
	return nil
}

func (self *Xgress) payloadIngester(payload *Payload) {
	if payload.IsSessionStartFlagSet() && self.rxerStarted.CompareAndSwap(false, true) {
		pfxlog.ContextLogger(self.Label()).WithFields(payload.GetLoggerFields()).Debug("received session start, starting xgress receiver")
		go self.rx()
	}

	start := time.Now()
	if !self.Options.RandomDrops || rand.Int31n(self.Options.Drop1InN) != 1 {
		self.PayloadReceived(payload)
	} else {
		pfxlog.ContextLogger(self.Label()).WithFields(payload.GetLoggerFields()).Error("drop!")
	}
	next := time.Now()
	payloadBufferTimer.Update(next.Sub(start))
	self.queueSends()
	payloadRelayTimer.UpdateSince(next)
}

func (self *Xgress) queueSends() {
	payload := self.linkRxBuffer.PeekHead()
	for payload != nil {
		select {
		case self.txQueue <- payload:
			self.linkRxBuffer.Remove(payload)
			payload = self.linkRxBuffer.PeekHead()
		default:
			payload = nil
		}
	}
}

func (self *Xgress) nextPayload() *Payload {
	select {
	case payload := <-self.txQueue:
		return payload
	default:
	}

	// nothing was availabe in the txQueue, request more, then wait on txQueue
	payloadIngester.payloadSendReq <- self

	select {
	case payload := <-self.txQueue:
		return payload
	case <-self.closeNotify:
	}

	// closed, check if there's anything pending in the queue
	select {
	case payload := <-self.txQueue:
		return payload
	default:
		return nil
	}
}

func (self *Xgress) tx() {
	log := pfxlog.ContextLogger(self.Label())

	log.Debug("started")
	defer log.Debug("exited")

	var payload *Payload

	for {
		payload = self.nextPayload()

		if payload == nil || payload.IsSessionEndFlagSet() {
			return
		}

		payloadLogger := log.WithFields(payload.GetLoggerFields())
		for _, peekHandler := range self.peekHandlers {
			peekHandler.Tx(self, payload)
		}
		if !payload.IsSessionStartFlagSet() && !payload.IsSessionEndFlagSet() {
			start := time.Now()
			n, err := self.peer.WritePayload(payload.Data, payload.Headers)
			if err != nil {
				payloadLogger.Warnf("write failed (%s), closing xgress", err)
				self.Close()
				return
			} else {
				payloadWriteTimer.UpdateSince(start)
				payloadLogger.Debugf("sent [%s]", info.ByteCount(int64(n)))
			}
		}
		payloadSize := len(payload.Data)
		size := atomic.AddUint32(&self.linkRxBuffer.size, ^uint32(payloadSize-1))
		pfxlog.Logger().Debugf("Payload %v of size %v removed from transmit buffer. New size: %v", payload.Sequence, payloadSize, size)
		localRecvBufferSizeBytesHistogram.Update(int64(size))

		lastBufferSizeSent := self.linkRxBuffer.getLastBufferSizeSent()
		if lastBufferSizeSent > 10000 && (lastBufferSizeSent>>1) > size {
			self.SendEmptyAck()
		}
	}
}

func (self *Xgress) rx() {
	log := pfxlog.ContextLogger(self.Label())

	log.Debugf("started with peer: %v", self.peer.LogContext())
	defer log.Debug("exited")

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("send on closed channel. error: (%v)", r)
			return
		}
	}()
	defer self.Close()

	for {
		buffer, headers, err := self.peer.ReadPayload()
		log.Debugf("read: %v bytes read", len(buffer))
		n := len(buffer)

		// if we got an EOF, but also some data, ignore the EOF, next read we'll get 0, EOF
		if err != nil && (n == 0 || err != io.EOF) {
			if err == io.EOF {
				log.Debugf("EOF, exiting xgress.rx loop")
			} else {
				log.Warnf("read failed (%s)", err)
			}

			return
		}

		if self.closed.Get() {
			return
		}
		start := 0
		remaining := n
		payloads := 0
		for {
			length := mathz.MinInt(remaining, int(self.Options.Mtu))
			payload := &Payload{
				Header: Header{
					SessionId: self.sessionId,
					Flags:     SetOriginatorFlag(0, self.originator),
				},
				Sequence: self.nextReceiveSequence(),
				Data:     buffer[start : start+length],
				Headers:  headers,
			}
			start += length
			remaining -= length
			payloads++

			self.forwardPayload(payload)
			payloadLogger := log.WithFields(payload.GetLoggerFields())
			payloadLogger.Debugf("received [%s]", info.ByteCount(int64(n)))

			if remaining < 1 {
				break
			}
		}

		logrus.Debugf("received [%d] payloads for [%d] bytes", payloads, n)
	}
}

func (self *Xgress) forwardPayload(payload *Payload) {
	sendCallback, err := self.payloadBuffer.BufferPayload(payload)

	if err != nil {
		pfxlog.Logger().WithError(err).Error("failure forwarding payload")
		return
	}

	for _, peekHandler := range self.peekHandlers {
		peekHandler.Rx(self, payload)
	}

	self.receiveHandler.HandleXgressReceive(payload, self)
	sendCallback()
}

func (self *Xgress) nextReceiveSequence() int32 {
	self.rxSequenceLock.Lock()
	defer self.rxSequenceLock.Unlock()

	next := self.rxSequence
	self.rxSequence++

	return next
}

func (self *Xgress) closeTimeoutHandler(duration time.Duration) {
	time.Sleep(duration)
	self.Close()
}

func (self *Xgress) PayloadReceived(payload *Payload) {
	log := pfxlog.Logger().WithFields(payload.GetLoggerFields())
	log.Debug("payload received")
	if self.linkRxBuffer.ReceiveUnordered(payload, self.Options.RxBufferSize) {
		log := pfxlog.Logger().WithFields(payload.GetLoggerFields())
		log.Debug("ready to acknowledge")

		ack := NewAcknowledgement(self.sessionId, self.originator)
		ack.RecvBufferSize = self.linkRxBuffer.Size()
		ack.Sequence = append(ack.Sequence, payload.Sequence)
		ack.RTT = payload.RTT

		atomic.StoreUint32(&self.linkRxBuffer.lastBufferSizeSent, ack.RecvBufferSize)
		acker.ack(ack, self.address)
	}
}

func (self *Xgress) SendEmptyAck() {
	pfxlog.Logger().WithField("session", self.sessionId).Debug("sending empty ack")
	ack := NewAcknowledgement(self.sessionId, self.originator)
	ack.RecvBufferSize = self.linkRxBuffer.Size()
	atomic.StoreUint32(&self.linkRxBuffer.lastBufferSizeSent, ack.RecvBufferSize)
	acker.ack(ack, self.address)
}
