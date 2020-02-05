/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/util/concurrenz"
	"github.com/netfoundry/ziti-foundation/util/info"
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
}

type Dialer interface {
	Dial(destination string, sessionId *identity.TokenId, address Address, bindHandler BindHandler) error
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
	options        *Options
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
		options:    options,
		txQueue:    make(chan *Payload),
		txBuffer:   NewTransmitBuffer(),
		rxSequence: 0,
	}
}

func (txg *Xgress) SessionId() *identity.TokenId {
	return txg.sessionId
}

func (txg *Xgress) Address() Address {
	return txg.address
}

func (txg *Xgress) Originator() Originator {
	return txg.originator
}

func (txg *Xgress) IsTerminator() bool {
	return txg.originator == Terminator
}

func (txg *Xgress) SetReceiveHandler(receiveHandler ReceiveHandler) {
	txg.receiveHandler = receiveHandler
}

func (txg *Xgress) SetPayloadBuffer(payloadBuffer *PayloadBuffer) {
	txg.payloadBuffer = payloadBuffer
}

func (txg *Xgress) SetCloseHandler(closeHandler CloseHandler) {
	txg.closeHandler = closeHandler
}

func (txg *Xgress) AddPeekHandler(peekHandler PeekHandler) {
	txg.peekHandlers = append(txg.peekHandlers, peekHandler)
}

func (txg *Xgress) Start() {
	go txg.tx()
	go txg.rx()
}

func (txg *Xgress) Label() string {
	return fmt.Sprintf("{s/%s|@/%s}<%s>", txg.sessionId.Token, string(txg.address), txg.originator.String())
}

func (txg *Xgress) GetEndSession() *Payload {
	endSession := &Payload{
		Header: Header{
			SessionId: txg.sessionId.Token,
			flags:     SetOriginatorFlag(uint32(PayloadFlagSessionEnd), txg.originator),
		},
		Sequence: txg.nextReceiveSequence(),
		Data:     nil,
	}
	return endSession
}

func (txg *Xgress) CloseTimeout(duration time.Duration) {
	go txg.closeTimeoutHandler(duration)
}

func (txg *Xgress) Close() {
	log := pfxlog.ContextLogger(txg.Label())
	log.Debug("closing xgress peer")
	if err := txg.peer.Close(); err != nil {
		log.WithError(err).Warn("error while closing xgress peer")
	}

	if txg.closed.CompareAndSwap(false, true) {
		log.Debug("closing tx queue")
		close(txg.txQueue)

		if txg.options.Retransmission && txg.payloadBuffer != nil {
			txg.payloadBuffer.Close()
		}

		for _, peekHandler := range txg.peekHandlers {
			peekHandler.Close(txg)
		}

		if txg.closeHandler != nil {
			txg.closeHandler.HandleXgressClose(txg)
		} else {
			pfxlog.ContextLogger(txg.Label()).Warn("no close handler")
		}
	} else {
		log.Debug("xgress already closed, skipping close")
	}
}

func (txg *Xgress) Closed() bool {
	return txg.closed.Get()
}

func (txg *Xgress) SendPayload(payload *Payload) error {
	defer func() {
		if r := recover(); r != nil {
			pfxlog.ContextLogger(txg.Label()).WithFields(payload.GetLoggerFields()).
				WithField("error", r).Error("send on closed channel")
			return
		}
	}()

	if payload.IsSessionEndFlagSet() {
		pfxlog.ContextLogger(txg.Label()).Error("received end of session Payload")
	}

	if !txg.closed.Get() {
		pfxlog.ContextLogger(txg.Label()).WithFields(payload.GetLoggerFields()).Debug("queuing to txQueue")
		txg.txQueue <- payload
	}
	return nil
}

func (txg *Xgress) SendAcknowledgement(acknowledgement *Acknowledgement) error {
	if txg.options.Retransmission && txg.payloadBuffer != nil {
		// if we have xgress <-> xgress in a single router, they will share a Payload buffer, as they share
		// a session and can get caught in deadlock if we don't dispatch to a new goroutine here
		go txg.payloadBuffer.ReceiveAcknowledgement(acknowledgement)
	}
	return nil
}

func (txg *Xgress) tx() {
	log := pfxlog.ContextLogger(txg.Label())

	log.Debug("started")
	defer log.Debug("exited")

	for {
		select {
		case inPayload := <-txg.txQueue:
			if inPayload != nil && !inPayload.IsSessionEndFlagSet() {
				payloadLogger := log.WithFields(inPayload.GetLoggerFields())
				if !txg.options.RandomDrops || rand.Int31n(txg.options.Drop1InN) != 1 {
					payloadLogger.Debug("adding to transmit buffer")
					txg.txBuffer.ReceiveUnordered(inPayload)
					if txg.options.Retransmission && txg.payloadBuffer != nil {
						payloadLogger.Debug("acknowledging")
						txg.payloadBuffer.AcknowledgePayload(inPayload)
					}
				} else {
					payloadLogger.Error("drop!")
				}

				ready := txg.txBuffer.ReadyForTransmit()
				for _, outPayload := range ready {
					outPayloadLogger := log.WithFields(outPayload.GetLoggerFields())
					for _, peekHandler := range txg.peekHandlers {
						peekHandler.Tx(txg, outPayload)
					}
					n, err := txg.peer.WritePayload(outPayload.Data, outPayload.Headers)
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

func (txg *Xgress) rx() {
	log := pfxlog.ContextLogger(txg.Label())

	log.Debug("started")
	defer log.Warn("exited")

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("send on closed channel. error: (%v)", r)
			return
		}
	}()
	defer txg.Close()

	for {
		buffer, headers, err := txg.peer.ReadPayload()
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
		if n < 1 && !txg.closed.Get() {
			continue
		}

		if !txg.closed.Get() {
			payload := &Payload{
				Header: Header{
					SessionId: txg.sessionId.Token,
					flags:     SetOriginatorFlag(0, txg.originator),
				},
				Sequence: txg.nextReceiveSequence(),
				Data:     buffer[:n],
				Headers:  headers,
			}
			payloadLogger := log.WithFields(payload.GetLoggerFields())

			if txg.options.Retransmission && txg.payloadBuffer != nil {
				payloadLogger.Debug("buffering payload")
				txg.payloadBuffer.BufferPayload(payload)
			}

			for _, peekHandler := range txg.peekHandlers {
				peekHandler.Rx(txg, payload)
			}

			txg.receiveHandler.HandleXgressReceive(payload, txg)

			payloadLogger.Debugf("received [%s]", info.ByteCount(int64(n)))
		} else {
			return
		}
	}
}

func (txg *Xgress) nextReceiveSequence() int32 {
	txg.rxSequenceLock.Lock()
	defer txg.rxSequenceLock.Unlock()

	next := txg.rxSequence
	txg.rxSequence++

	return next
}

func (txg *Xgress) closeTimeoutHandler(duration time.Duration) {
	time.Sleep(duration)
	txg.Close()
}
