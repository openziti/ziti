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
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/info"
	"github.com/openziti/foundation/util/mathz"
	"github.com/sirupsen/logrus"
	"io"
	"math/rand"
	"sync"
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
	Dial(destination string, sessionId *identity.TokenId, address Address, bindHandler BindHandler) error
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
	HandleXgressBind(sessionId *identity.TokenId, address Address, originator Originator, x *Xgress)
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
	sessionId      *identity.TokenId
	address        Address
	peer           Connection
	originator     Originator
	Options        *Options
	txQueue        chan *Payload
	txBuffer       *TransmitBuffer
	rxSequence     int32
	rxSequenceLock sync.Mutex
	receiveHandler ReceiveHandler
	payloadBuffer  *PayloadBuffer
	closed         concurrenz.AtomicBoolean
	closeHandler   CloseHandler
	peekHandlers   []PeekHandler
}

func NewXgress(sessionId *identity.TokenId, address Address, peer Connection, originator Originator, options *Options) *Xgress {
	return &Xgress{
		sessionId:  sessionId,
		address:    address,
		peer:       peer,
		originator: originator,
		Options:    options,
		txQueue:    make(chan *Payload),
		txBuffer:   NewTransmitBuffer(),
		rxSequence: 0,
	}
}

func (self *Xgress) SessionId() *identity.TokenId {
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

func (self *Xgress) SetPayloadBuffer(payloadBuffer *PayloadBuffer) {
	self.payloadBuffer = payloadBuffer
}

func (self *Xgress) SetCloseHandler(closeHandler CloseHandler) {
	self.closeHandler = closeHandler
}

func (self *Xgress) AddPeekHandler(peekHandler PeekHandler) {
	self.peekHandlers = append(self.peekHandlers, peekHandler)
}

func (self *Xgress) Start() {
	go self.tx()
	go self.rx()
}

func (self *Xgress) Label() string {
	return fmt.Sprintf("{s/%s|@/%s}<%s>", self.sessionId.Token, string(self.address), self.originator.String())
}

func (self *Xgress) GetEndSession() *Payload {
	endSession := &Payload{
		Header: Header{
			SessionId: self.sessionId.Token,
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
		close(self.txQueue)

		if self.Options.Retransmission && self.payloadBuffer != nil {
			self.payloadBuffer.Close()
		}

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
	defer func() {
		if r := recover(); r != nil {
			pfxlog.ContextLogger(self.Label()).WithFields(payload.GetLoggerFields()).
				WithField("error", r).Error("send on closed channel")
			return
		}
	}()

	if payload.IsSessionEndFlagSet() {
		pfxlog.ContextLogger(self.Label()).Debug("received end of session Payload")
	}

	if !self.closed.Get() {
		pfxlog.ContextLogger(self.Label()).WithFields(payload.GetLoggerFields()).Debug("queuing to txQueue")
		self.txQueue <- payload
	}
	return nil
}

func (self *Xgress) SendAcknowledgement(acknowledgement *Acknowledgement) error {
	if self.Options.Retransmission && self.payloadBuffer != nil {
		// if we have xgress <-> xgress in a single router, they will share a Payload buffer, as they share
		// a session and can get caught in deadlock if we don't dispatch to a new goroutine here
		go self.payloadBuffer.ReceiveAcknowledgement(acknowledgement)
	}
	return nil
}

func (self *Xgress) tx() {
	log := pfxlog.ContextLogger(self.Label())

	log.Debug("started")
	defer log.Debug("exited")

	for {
		select {
		case inPayload := <-self.txQueue:
			if inPayload != nil && !inPayload.IsSessionEndFlagSet() {
				payloadLogger := log.WithFields(inPayload.GetLoggerFields())
				if !self.Options.RandomDrops || rand.Int31n(self.Options.Drop1InN) != 1 {
					payloadLogger.Debug("adding to transmit buffer")
					self.txBuffer.ReceiveUnordered(inPayload)
					if self.Options.Retransmission && self.payloadBuffer != nil {
						payloadLogger.Debug("acknowledging")
						self.payloadBuffer.AcknowledgePayload(inPayload)
					}
				} else {
					payloadLogger.Error("drop!")
				}

				ready := self.txBuffer.ReadyForTransmit()
				for _, outPayload := range ready {
					outPayloadLogger := log.WithFields(outPayload.GetLoggerFields())
					for _, peekHandler := range self.peekHandlers {
						peekHandler.Tx(self, outPayload)
					}
					n, err := self.peer.WritePayload(outPayload.Data, outPayload.Headers)
					if err != nil {
						outPayloadLogger.Warnf("write failed (%s)", err)
					} else {
						outPayloadLogger.Debugf("sent (#%d) [%s]", outPayload.GetSequence(), info.ByteCount(int64(n)))
					}
				}

				if len(ready) < 1 {
					payloadLogger.Debug("queued transmit")
				}

			} else {
				return
			}
		}
	}
}

func (self *Xgress) rx() {
	log := pfxlog.ContextLogger(self.Label())

	log.Debug("started")
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
		if n < 1 && !self.closed.Get() {
			continue
		}

		if !self.closed.Get() {
			start := 0
			remaining := n
			payloads := 0
			for remaining > 0 {
				length := mathz.MinInt(remaining, int(self.Options.Mtu))
				payload := &Payload{
					Header: Header{
						SessionId: self.sessionId.Token,
						Flags:     SetOriginatorFlag(0, self.originator),
					},
					Sequence: self.nextReceiveSequence(),
					Data:     buffer[start : start+length],
					Headers:  headers,
				}
				start += length
				remaining -= length
				payloads++

				payloadLogger := log.WithFields(payload.GetLoggerFields())

				if self.Options.Retransmission && self.payloadBuffer != nil {
					//payloadLogger.Debug("buffering payload")
					self.payloadBuffer.BufferPayload(payload)
				}

				for _, peekHandler := range self.peekHandlers {
					peekHandler.Rx(self, payload)
				}

				self.receiveHandler.HandleXgressReceive(payload, self)

				payloadLogger.Debugf("received [%s]", info.ByteCount(int64(n)))
			}

			logrus.Debugf("received [%d] payloads for [%d] bytes", payloads, n)

		} else {
			return
		}
	}
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
