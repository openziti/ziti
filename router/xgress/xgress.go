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
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/openziti/ziti/controller/xt"
	"github.com/sirupsen/logrus"
)

const (
	HeaderKeyUUID = 0

	closedFlag            = 0
	rxerStartedFlag       = 1
	endOfCircuitRecvdFlag = 2
	endOfCircuitSentFlag  = 3
)

type Address string

type Listener interface {
	Listen(address string, bindHandler BindHandler) error
	Close() error
}

type DialParams interface {
	GetCtrlId() string
	GetDestination() string
	GetCircuitId() *identity.TokenId
	GetAddress() Address
	GetBindHandler() BindHandler
	GetLogContext() logcontext.Context
	GetDeadline() time.Time
	GetCircuitTags() map[string]string
}

type Dialer interface {
	Dial(params DialParams) (xt.PeerData, error)
	IsTerminatorValid(id string, destination string) bool
}

type InspectableDialer interface {
	Dialer
	InspectTerminator(id string, destination string, fixInvalid bool) (bool, string)
}

type Inspectable interface {
	Inspect(key string, timeout time.Duration) any
}

type Factory interface {
	CreateListener(optionsData OptionsData) (Listener, error)
	CreateDialer(optionsData OptionsData) (Dialer, error)
}

type OptionsData map[interface{}]interface{}

// The BindHandlers are invoked to install the appropriate handlers.
type BindHandler interface {
	HandleXgressBind(x *Xgress)
}

type ControlReceiver interface {
	HandleControlReceive(controlType ControlType, headers channel.Headers)
}

// ReceiveHandler is invoked by an xgress whenever data is received from the connected peer. Generally a ReceiveHandler
// is implemented to connect the xgress to a data plane data transmission system.
type ReceiveHandler interface {
	// HandleXgressReceive is invoked when data is received from the connected xgress peer.
	//
	HandleXgressReceive(payload *Payload, x *Xgress)
	HandleControlReceive(control *Control, x *Xgress)
}

// CloseHandler is invoked by an xgress when the connected peer terminates the communication.
type CloseHandler interface {
	// HandleXgressClose is invoked when the connected peer terminates the communication.
	//
	HandleXgressClose(x *Xgress)
}

// CloseHandlerF is the function version of CloseHandler
type CloseHandlerF func(x *Xgress)

func (self CloseHandlerF) HandleXgressClose(x *Xgress) {
	self(x)
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
	HandleControlMsg(controlType ControlType, headers channel.Headers, responder ControlReceiver) error
}

type Xgress struct {
	circuitId            string
	ctrlId               string
	address              Address
	peer                 Connection
	originator           Originator
	Options              *Options
	txQueue              chan *Payload
	closeNotify          chan struct{}
	rxSequence           int32
	rxSequenceLock       sync.Mutex
	receiveHandler       ReceiveHandler
	payloadBuffer        *LinkSendBuffer
	linkRxBuffer         *LinkReceiveBuffer
	closeHandlers        []CloseHandler
	peekHandlers         []PeekHandler
	flags                concurrenz.AtomicBitSet
	timeOfLastRxFromLink int64
	tags                 map[string]string
}

func (self *Xgress) GetIntervalId() string {
	return self.circuitId
}

func (self *Xgress) GetTags() map[string]string {
	return self.tags
}

func NewXgress(circuitId string, ctrlId string, address Address, peer Connection, originator Originator, options *Options, tags map[string]string) *Xgress {
	result := &Xgress{
		circuitId:            circuitId,
		ctrlId:               ctrlId,
		address:              address,
		peer:                 peer,
		originator:           originator,
		Options:              options,
		txQueue:              make(chan *Payload, options.TxQueueSize),
		closeNotify:          make(chan struct{}),
		rxSequence:           0,
		linkRxBuffer:         NewLinkReceiveBuffer(),
		timeOfLastRxFromLink: info.NowInMilliseconds(),
		tags:                 tags,
	}
	result.payloadBuffer = NewLinkSendBuffer(result)
	return result
}

func (self *Xgress) GetTimeOfLastRxFromLink() int64 {
	return atomic.LoadInt64(&self.timeOfLastRxFromLink)
}

func (self *Xgress) CircuitId() string {
	return self.circuitId
}

func (self *Xgress) CtrlId() string {
	return self.ctrlId
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

func (self *Xgress) AddCloseHandler(closeHandler CloseHandler) {
	self.closeHandlers = append(self.closeHandlers, closeHandler)
}

func (self *Xgress) AddPeekHandler(peekHandler PeekHandler) {
	self.peekHandlers = append(self.peekHandlers, peekHandler)
}

func (self *Xgress) IsEndOfCircuitReceived() bool {
	return self.flags.IsSet(endOfCircuitRecvdFlag)
}

func (self *Xgress) markCircuitEndReceived() {
	self.flags.Set(endOfCircuitRecvdFlag, true)
}

func (self *Xgress) IsCircuitStarted() bool {
	return !self.IsTerminator() || self.flags.IsSet(rxerStartedFlag)
}

func (self *Xgress) firstCircuitStartReceived() bool {
	return self.flags.CompareAndSet(rxerStartedFlag, false, true)
}

func (self *Xgress) Start() {
	log := pfxlog.ContextLogger(self.Label())
	if self.IsTerminator() {
		log.Debug("terminator: waiting for circuit start before starting receiver")
		if self.Options.CircuitStartTimeout > time.Second {
			time.AfterFunc(self.Options.CircuitStartTimeout, self.terminateIfNotStarted)
		}
	} else {
		log.Debug("initiator: sending circuit start")
		self.forwardPayload(self.GetStartCircuit())
		go self.rx()
	}
	go self.tx()
}

func (self *Xgress) terminateIfNotStarted() {
	if !self.IsCircuitStarted() {
		logrus.WithField("xgress", self.Label()).Warn("xgress circuit not started in time, closing")
		self.Close()
	}
}

func (self *Xgress) Label() string {
	return fmt.Sprintf("{c/%s|@/%s}<%s>", self.circuitId, string(self.address), self.originator.String())
}

func (self *Xgress) GetStartCircuit() *Payload {
	startCircuit := &Payload{
		Header: Header{
			CircuitId: self.circuitId,
			Flags:     SetOriginatorFlag(uint32(PayloadFlagCircuitStart), self.originator),
		},
		Sequence: self.nextReceiveSequence(),
		Data:     nil,
	}
	return startCircuit
}

func (self *Xgress) GetEndCircuit() *Payload {
	endCircuit := &Payload{
		Header: Header{
			CircuitId: self.circuitId,
			Flags:     SetOriginatorFlag(uint32(PayloadFlagCircuitEnd), self.originator),
		},
		Sequence: self.nextReceiveSequence(),
		Data:     nil,
	}
	return endCircuit
}

func (self *Xgress) ForwardEndOfCircuit(sendF func(payload *Payload) bool) {
	// for now always send end of circuit. too many is better than not enough
	if !self.IsEndOfCircuitSent() {
		sendF(self.GetEndCircuit())
		self.flags.Set(endOfCircuitSentFlag, true)
	}
}

func (self *Xgress) IsEndOfCircuitSent() bool {
	return self.flags.IsSet(endOfCircuitSentFlag)
}

func (self *Xgress) CloseTimeout(duration time.Duration) {
	if self.payloadBuffer.CloseWhenEmpty() { // If we clear the send buffer, close sooner
		time.AfterFunc(duration, self.Close)
	}
}

func (self *Xgress) Unrouted() {
	// When we're unrouted, if end of circuit hasn't already arrived, give incoming/queued data
	// a chance to outflow before closing
	if !self.flags.IsSet(closedFlag) {
		self.payloadBuffer.Close()
		time.AfterFunc(self.Options.MaxCloseWait, self.Close)
	}
}

/*
Things which can trigger close

1. Read fails
2. Write fails
3. End of Circuit received
4. Unroute received
*/
func (self *Xgress) Close() {
	log := pfxlog.ContextLogger(self.Label())

	if self.flags.CompareAndSet(closedFlag, false, true) {
		log.Debug("closing xgress peer")
		if err := self.peer.Close(); err != nil {
			log.WithError(err).Warn("error while closing xgress peer")
		}

		log.Debug("closing tx queue")
		close(self.closeNotify)

		self.payloadBuffer.Close()

		for _, peekHandler := range self.peekHandlers {
			peekHandler.Close(self)
		}

		if len(self.closeHandlers) != 0 {
			for _, closeHandler := range self.closeHandlers {
				closeHandler.HandleXgressClose(self)
			}
		} else {
			pfxlog.ContextLogger(self.Label()).Warn("no close handler")
		}
	}
}

func (self *Xgress) Closed() bool {
	return self.flags.IsSet(closedFlag)
}

func (self *Xgress) SendPayload(payload *Payload) error {
	if self.Closed() {
		return nil
	}

	if payload.IsCircuitEndFlagSet() {
		pfxlog.ContextLogger(self.Label()).Debug("received end of circuit Payload")
	}
	atomic.StoreInt64(&self.timeOfLastRxFromLink, info.NowInMilliseconds())
	payloadIngester.ingest(payload, self)

	return nil
}

func (self *Xgress) SendAcknowledgement(acknowledgement *Acknowledgement) error {
	ackRxMeter.Mark(1)
	self.payloadBuffer.ReceiveAcknowledgement(acknowledgement)
	return nil
}

func (self *Xgress) SendControl(control *Control) error {
	return self.peer.HandleControlMsg(control.Type, control.Headers, self)
}

func (self *Xgress) HandleControlReceive(controlType ControlType, headers channel.Headers) {
	control := &Control{
		Type:      controlType,
		CircuitId: self.circuitId,
		Headers:   headers,
	}
	self.receiveHandler.HandleControlReceive(control, self)
}

func (self *Xgress) payloadIngester(payload *Payload) {
	if payload.IsCircuitStartFlagSet() && self.firstCircuitStartReceived() {
		pfxlog.ContextLogger(self.Label()).WithFields(payload.GetLoggerFields()).Debug("received circuit start, starting xgress receiver")
		go self.rx()
	}

	if !self.Options.RandomDrops || rand.Int31n(self.Options.Drop1InN) != 1 {
		self.PayloadReceived(payload)
	} else {
		pfxlog.ContextLogger(self.Label()).WithFields(payload.GetLoggerFields()).Error("drop!")
	}
	self.queueSends()
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

	// nothing was available in the txQueue, request more, then wait on txQueue
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
	defer func() {
		if self.IsEndOfCircuitReceived() {
			self.Close()
		} else {
			self.flushSendThenClose()
		}
	}()

	clearPayloadFromSendBuffer := func(payload *Payload) {
		payloadSize := len(payload.Data)
		size := atomic.AddUint32(&self.linkRxBuffer.size, ^uint32(payloadSize-1)) // subtraction for uint32

		payloadLogger := log.WithFields(payload.GetLoggerFields())
		payloadLogger.Debugf("payload %v of size %v removed from rx buffer, new size: %v", payload.Sequence, payloadSize, size)

		lastBufferSizeSent := self.linkRxBuffer.getLastBufferSizeSent()
		if lastBufferSizeSent > 10000 && (lastBufferSizeSent>>1) > size {
			self.SendEmptyAck()
		}
	}

	sendPayload := func(payload *Payload) bool {
		payloadLogger := log.WithFields(payload.GetLoggerFields())

		if payload.IsCircuitEndFlagSet() {
			self.markCircuitEndReceived()
			payloadLogger.Debug("circuit end payload received, exiting")
			return false
		}

		payloadLogger.Debug("sending")

		for _, peekHandler := range self.peekHandlers {
			peekHandler.Tx(self, payload)
		}

		if !payload.IsCircuitStartFlagSet() {
			start := time.Now()
			n, err := self.peer.WritePayload(payload.Data, payload.Headers)
			if err != nil {
				payloadLogger.Warnf("write failed (%s), closing xgress", err)
				self.Close()
				return false
			} else {
				payloadWriteTimer.UpdateSince(start)
				payloadLogger.Infof("payload sent [%s]", info.ByteCount(int64(n)))
			}
		}
		return true
	}

	var payload *Payload
	var payloadChunk *Payload

	payloadStarted := false
	payloadComplete := false
	var payloadSize int64
	var payloadWriteOffset int

	for {
		payloadChunk = self.nextPayload()

		if payloadChunk == nil {
			log.Debug("nil payload received, exiting")
			return
		}

		if !isPayloadFlagSet(payloadChunk.GetFlags(), PayloadFlagChunk) {
			if !sendPayload(payloadChunk) {
				return
			}
			clearPayloadFromSendBuffer(payloadChunk)
			continue
		}

		var payloadReadOffset int
		if !payloadStarted {
			payloadSize, payloadReadOffset = binary.Varint(payloadChunk.Data)

			if len(payloadChunk.Data) == 0 || payloadSize+int64(payloadReadOffset) == int64(len(payloadChunk.Data)) {
				payload = payloadChunk
				payload.Data = payload.Data[payloadReadOffset:]
				payloadComplete = true
			} else {
				payload = &Payload{
					Header:   payloadChunk.Header,
					Sequence: payloadChunk.Sequence,
					Headers:  payloadChunk.Headers,
					Data:     make([]byte, payloadSize),
				}
			}
			payloadStarted = true
		}

		if !payloadComplete {
			chunkData := payloadChunk.Data[payloadReadOffset:]
			copy(payload.Data[payloadWriteOffset:], chunkData)
			payloadWriteOffset += len(chunkData)
			payloadComplete = int64(payloadWriteOffset) == payloadSize
		}

		payloadLogger := log.WithFields(payload.GetLoggerFields())
		payloadLogger.Debugf("received payload chunk. seq: %d, first: %v, complete: %v, chunk size: %d, payload size: %d, writeOffset: %d",
			payloadChunk.Sequence, len(payload.Data) == 0 || payloadReadOffset > 0, payloadComplete, len(payloadChunk.Data), payloadSize, payloadWriteOffset)

		if !payloadComplete {
			clearPayloadFromSendBuffer(payloadChunk)
			continue
		}

		payloadStarted = false
		payloadComplete = false
		payloadWriteOffset = 0

		if !sendPayload(payload) {
			return
		}
		clearPayloadFromSendBuffer(payloadChunk)
	}
}

func (self *Xgress) flushSendThenClose() {
	self.CloseTimeout(self.Options.MaxCloseWait)
	self.ForwardEndOfCircuit(func(payload *Payload) bool {
		if self.payloadBuffer.closed.Load() {
			// Avoid spurious 'failed to forward payload' error if the buffer is already closed
			return false
		}

		pfxlog.ContextLogger(self.Label()).Info("sending end of circuit payload")
		return self.forwardPayload(payload)
	})
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

	defer self.flushSendThenClose()

	for {
		buffer, headers, err := self.peer.ReadPayload()
		log.Debugf("payload read: %d bytes read", len(buffer))
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

		if self.Closed() {
			return
		}

		if n < int(self.Options.Mtu) || self.Options.Mtu == 0 {
			if !self.sendUnchunkedBuffer(buffer, headers) {
				return
			}
			continue
		}

		first := true
		for len(buffer) > 0 {
			chunk := make([]byte, self.Options.Mtu)
			dataTarget := chunk
			offset := 0
			if first {
				offset = binary.PutVarint(chunk, int64(n))
				dataTarget = chunk[offset:]
			}

			written := copy(dataTarget, buffer)
			buffer = buffer[written:]

			payload := &Payload{
				Header: Header{
					CircuitId: self.circuitId,
					Flags:     setPayloadFlag(SetOriginatorFlag(0, self.originator), PayloadFlagChunk),
				},
				Sequence: self.nextReceiveSequence(),
				Data:     chunk[:offset+written],
			}

			if first {
				payload.Headers = headers
			}
			log.Debugf("sending payload chunk. seq: %d, first: %v, chunk size: %d, payload size: %d, remainder: %d", payload.Sequence, first, len(payload.Data), n, len(buffer))
			first = false

			// if the payload buffer is closed, we can't forward any more data, so might as well exit the rx loop
			// The txer will still have a chance to flush any already received data
			if !self.forwardPayload(payload) {
				return
			}

			payloadLogger := log.WithFields(payload.GetLoggerFields())
			payloadLogger.Debugf("forwarded [%s]", info.ByteCount(int64(n)))
		}

		logrus.Debugf("received payload for [%d] bytes", n)
	}
}

func (self *Xgress) sendUnchunkedBuffer(buf []byte, headers map[uint8][]byte) bool {
	log := pfxlog.ContextLogger(self.Label())

	payload := &Payload{
		Header: Header{
			CircuitId: self.circuitId,
			Flags:     SetOriginatorFlag(0, self.originator),
		},
		Sequence: self.nextReceiveSequence(),
		Data:     buf,
		Headers:  headers,
	}

	log.Debugf("sending unchunked payload. seq: %d, payload size: %d", payload.Sequence, len(payload.Data))

	// if the payload buffer is closed, we can't forward any more data, so might as well exit the rx loop
	// The txer will still have a chance to flush any already received data
	if !self.forwardPayload(payload) {
		return false
	}

	payloadLogger := log.WithFields(payload.GetLoggerFields())
	payloadLogger.Debugf("forwarded [%s]", info.ByteCount(int64(len(buf))))
	return true
}

func (self *Xgress) forwardPayload(payload *Payload) bool {
	sendCallback, err := self.payloadBuffer.BufferPayload(payload)

	if err != nil {
		pfxlog.ContextLogger(self.Label()).WithError(err).Error("failure to buffer payload")
		return false
	}

	for _, peekHandler := range self.peekHandlers {
		peekHandler.Rx(self, payload)
	}

	self.receiveHandler.HandleXgressReceive(payload, self)
	sendCallback()
	return true
}

func (self *Xgress) nextReceiveSequence() int32 {
	self.rxSequenceLock.Lock()
	defer self.rxSequenceLock.Unlock()

	next := self.rxSequence
	self.rxSequence++

	return next
}

func (self *Xgress) PayloadReceived(payload *Payload) {
	log := pfxlog.ContextLogger(self.Label()).WithFields(payload.GetLoggerFields())
	log.Debug("payload received")
	if self.originator == payload.GetOriginator() {
		// a payload sent from this xgress has arrived back at this xgress, instead of the other end
		log.Warn("ouroboros (circuit cycle) detected, dropping payload")
	} else if self.linkRxBuffer.ReceiveUnordered(payload, self.Options.RxBufferSize) {
		log.Debug("ready to acknowledge")

		ack := NewAcknowledgement(self.circuitId, self.originator)
		ack.RecvBufferSize = self.linkRxBuffer.Size()
		ack.Sequence = append(ack.Sequence, payload.Sequence)
		ack.RTT = payload.RTT

		atomic.StoreUint32(&self.linkRxBuffer.lastBufferSizeSent, ack.RecvBufferSize)
		acker.ack(ack, self.address)
	} else {
		log.Debug("dropped")
	}
}

func (self *Xgress) SendEmptyAck() {
	pfxlog.ContextLogger(self.Label()).WithField("circuit", self.circuitId).Debug("sending empty ack")
	ack := NewAcknowledgement(self.circuitId, self.originator)
	ack.RecvBufferSize = self.linkRxBuffer.Size()
	atomic.StoreUint32(&self.linkRxBuffer.lastBufferSizeSent, ack.RecvBufferSize)
	acker.ack(ack, self.address)
}

func (self *Xgress) GetSequence() int32 {
	self.rxSequenceLock.Lock()
	defer self.rxSequenceLock.Unlock()
	return self.rxSequence
}

func (self *Xgress) InspectCircuit(detail *inspect.CircuitInspectDetail) {
	timeSinceLastRxFromLink := time.Duration(info.NowInMilliseconds()-atomic.LoadInt64(&self.timeOfLastRxFromLink)) * time.Millisecond
	xgressDetail := &inspect.XgressDetail{
		Address:               string(self.address),
		Originator:            self.originator.String(),
		TimeSinceLastLinkRx:   timeSinceLastRxFromLink.String(),
		SendBufferDetail:      self.payloadBuffer.Inspect(),
		RecvBufferDetail:      self.linkRxBuffer.Inspect(),
		XgressPointer:         fmt.Sprintf("%p", self),
		LinkSendBufferPointer: fmt.Sprintf("%p", self.payloadBuffer),
		Sequence:              self.GetSequence(),
		Flags:                 strconv.FormatUint(uint64(self.flags.Load()), 2),
	}

	detail.XgressDetails[string(self.address)] = xgressDetail

	if detail.IncludeGoroutines() {
		xgressDetail.Goroutines = self.getRelatedGoroutines(xgressDetail.XgressPointer, xgressDetail.LinkSendBufferPointer)
	}
}

func (self *Xgress) getRelatedGoroutines(contains ...string) []string {
	reader := bytes.NewBufferString(debugz.GenerateStack())
	scanner := bufio.NewScanner(reader)
	var result []string
	var buf *bytes.Buffer
	xgressRelated := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "goroutine") && strings.HasSuffix(line, ":") {
			result = self.addGoroutineIfRelated(buf, xgressRelated, result, contains...)
			buf = &bytes.Buffer{}
			xgressRelated = false
		}

		if buf != nil {
			if strings.Contains(line, "xgress") {
				xgressRelated = true
			}
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	result = self.addGoroutineIfRelated(buf, xgressRelated, result, contains...)
	if err := scanner.Err(); err != nil {
		result = append(result, "goroutine parsing error: %v", err.Error())
	}
	return result
}

func (self *Xgress) addGoroutineIfRelated(buf *bytes.Buffer, xgressRelated bool, result []string, contains ...string) []string {
	if !xgressRelated {
		return result
	}
	if buf != nil {
		gr := buf.String()
		// ignore the current goroutine
		if strings.Contains(gr, "GenerateStack") {
			return result
		}

		for _, s := range contains {
			if strings.Contains(gr, s) {
				result = append(result, gr)
				break
			}
		}
	}
	return result
}
