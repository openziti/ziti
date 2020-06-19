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
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/info"
	"github.com/orcaman/concurrent-map"
	"github.com/sirupsen/logrus"
	"time"
)

type PayloadBufferForwarder interface {
	ForwardPayload(srcAddr Address, payload *Payload) error
	ForwardAcknowledgement(srcAddr Address, acknowledgement *Acknowledgement) error
}

type PayloadBufferController struct {
	buffers   cmap.ConcurrentMap // map[bufferId{address+sessionId}]*PayloadBuffer
	sessions  cmap.ConcurrentMap // map[sessionId][]string{bufferId{address+sessionId}
	forwarder PayloadBufferForwarder
}

func NewPayloadBufferController(forwarder PayloadBufferForwarder) *PayloadBufferController {
	return &PayloadBufferController{
		buffers:   cmap.New(),
		sessions:  cmap.New(),
		forwarder: forwarder,
	}
}

func (controller *PayloadBufferController) BufferForSession(sessionId *identity.TokenId, address Address) *PayloadBuffer {
	bufferId := string(address) + sessionId.Token
	if i, found := controller.buffers.Get(bufferId); found {
		return i.(*PayloadBuffer)
	}

	buffer := NewPayloadBuffer(sessionId, controller.forwarder)
	controller.buffers.Set(bufferId, buffer)
	controller.sessions.Upsert(sessionId.Token, bufferId, func(exists bool, valueInMap interface{}, newValue interface{}) interface{} {
		nv := newValue.(string)
		if !exists {
			return []string{nv}
		}
		res := valueInMap.([]string)
		return append(res, nv)
	})

	return buffer
}

func (controller *PayloadBufferController) EndSession(sessionId *identity.TokenId) {
	if v, found := controller.sessions.Get(sessionId.Token); found {
		logrus.Debugf("cleaning up for [s/%s]", sessionId.Token)
		bufferIds := v.([]string)
		for _, bufferId := range bufferIds {
			logrus.Debugf("removing bufferId [%s]", bufferId)
			controller.buffers.Remove(bufferId)
		}
		controller.sessions.Remove(sessionId.Token)
	}
}

type PayloadBuffer struct {
	sessionId         *identity.TokenId
	buffer            map[int32]*payloadAge
	newlyBuffered     chan *Payload
	acked             map[int32]int64
	lastAck           int64
	newlyAcknowledged chan *Payload
	newlyReceivedAcks chan *Acknowledgement
	receivedAckHwm    int32
	forwarder         PayloadBufferForwarder
	SrcAddress        Address
	Originator        Originator

	config struct {
		retransmitAge int64
		ackPeriod     int64
		ackCount      int
		idleAckAfter  int64
	}
}

type payloadAge struct {
	payload *Payload
	age     int64
}

func NewPayloadBuffer(sessionId *identity.TokenId, forwarder PayloadBufferForwarder) *PayloadBuffer {
	buffer := &PayloadBuffer{
		sessionId:         sessionId,
		buffer:            make(map[int32]*payloadAge),
		newlyBuffered:     make(chan *Payload),
		acked:             make(map[int32]int64),
		lastAck:           info.NowInMilliseconds(),
		newlyAcknowledged: make(chan *Payload),
		newlyReceivedAcks: make(chan *Acknowledgement),
		receivedAckHwm:    -1,
		forwarder:         forwarder,
	}

	buffer.config.retransmitAge = 2000
	buffer.config.ackPeriod = 1000
	buffer.config.ackCount = 96
	buffer.config.idleAckAfter = 5000

	go buffer.run()
	return buffer
}

func (buffer *PayloadBuffer) BufferPayload(payload *Payload) {
	defer func() {
		if r := recover(); r != nil {
			pfxlog.ContextLogger("s/" + buffer.sessionId.Token).Error("send on closed channel")
		}
	}()
	buffer.newlyBuffered <- payload
	pfxlog.ContextLogger("s/"+payload.GetSessionId()).Debugf("buffered [%d]", payload.GetSequence())
}

func (buffer *PayloadBuffer) AcknowledgePayload(payload *Payload) {
	defer func() {
		if r := recover(); r != nil {
			pfxlog.ContextLogger("s/" + buffer.sessionId.Token).Error("send on closed channel")
		}
	}()
	buffer.newlyAcknowledged <- payload
	pfxlog.ContextLogger("s/"+payload.GetSessionId()).Debugf("acknowledge [%d]", payload.GetSequence())
}

func (buffer *PayloadBuffer) ReceiveAcknowledgement(ack *Acknowledgement) {
	defer func() {
		if r := recover(); r != nil {
			pfxlog.ContextLogger("s/" + buffer.sessionId.Token).Error("send on closed channel")
		}
	}()
	buffer.newlyReceivedAcks <- ack
	pfxlog.ContextLogger("s/"+ack.SessionId).Debugf("received ack [%d]", len(ack.Sequence))
}

func (buffer *PayloadBuffer) Close() {
	logrus.Debugf("[%p] closing", buffer)
	defer func() {
		if r := recover(); r != nil {
			pfxlog.Logger().Debug("already closed")
		}
	}()
	close(buffer.newlyBuffered)
	close(buffer.newlyAcknowledged)
	close(buffer.newlyReceivedAcks)
}

func (buffer *PayloadBuffer) run() {
	log := pfxlog.ContextLogger("s/" + buffer.sessionId.Token)
	defer log.Debugf("[%p] exited", buffer)
	log.Debugf("[%p] started", buffer)

	lastDebug := info.NowInMilliseconds()
	for {
		now := info.NowInMilliseconds()
		if now-lastDebug >= 2000 {
			buffer.debug(now)
			lastDebug = now
		}

		select {
		case ack := <-buffer.newlyReceivedAcks:
			if ack != nil {
				if err := buffer.receiveAcknowledgement(ack); err != nil {
					log.Errorf("unexpected error (%s)", err)
				}
			} else {
				return
			}
			if err := buffer.retransmit(); err != nil {
				log.Errorf("unexpected error retransmitting (%s)", err)
			}

		case payload := <-buffer.newlyAcknowledged:
			if payload != nil {
				if err := buffer.acknowledgePayload(payload); err != nil {
					log.Errorf("unexpected error (%s)", err)
				}
				if err := buffer.acknowledge(); err != nil {
					log.Errorf("unexpected error (%s)", err)
				}
			} else {
				return
			}

		case payload := <-buffer.newlyBuffered:
			if payload != nil {
				if err := buffer.bufferPayload(payload); err != nil {
					log.Errorf("unexpected error (%s)", err)
				}
				if err := buffer.acknowledge(); err != nil {
					log.Errorf("unexpected error (%s)", err)
				}
			} else {
				return
			}

		case <-time.After(time.Duration(buffer.config.idleAckAfter) * time.Millisecond):
			if err := buffer.acknowledge(); err != nil {
				log.Errorf("unexpected error acknowledging (%s)", err)
			}
			if err := buffer.retransmit(); err != nil {
				log.Errorf("unexpected error retransmitting (%s)", err)
			}
		}
	}
}

func (buffer *PayloadBuffer) bufferPayload(payload *Payload) error {
	if buffer.sessionId.Token == payload.GetSessionId() {
		buffer.buffer[payload.GetSequence()] = &payloadAge{payload: payload, age: info.NowInMilliseconds()}

	} else {
		return errors.New("unexpected Payload")
	}

	return nil
}

func (buffer *PayloadBuffer) acknowledgePayload(payload *Payload) error {
	if buffer.sessionId.Token == payload.SessionId {
		buffer.acked[payload.Sequence] = info.NowInMilliseconds()

	} else {
		return errors.New("unexpected Payload")
	}

	return nil
}

func (buffer *PayloadBuffer) receiveAcknowledgement(ack *Acknowledgement) error {
	log := pfxlog.ContextLogger("s/" + buffer.sessionId.Token)
	if buffer.sessionId.Token == ack.SessionId {
		for _, sequence := range ack.Sequence {
			if sequence > buffer.receivedAckHwm {
				buffer.receivedAckHwm = sequence
			}
			delete(buffer.buffer, sequence)
			log.Debugf("acknowledged sequence [%d]", sequence)
		}

	} else {
		return errors.New("unexpected acknowledgement")
	}
	return nil
}

func (buffer *PayloadBuffer) acknowledge() error {
	log := pfxlog.ContextLogger("s/" + buffer.sessionId.Token)
	now := info.NowInMilliseconds()

	if now-buffer.lastAck >= buffer.config.ackPeriod || len(buffer.acked) >= buffer.config.ackCount {
		log.Debug("ready to acknowledge")

		ack := NewAcknowledgement(buffer.sessionId.Token, buffer.Originator)
		for sequence := range buffer.acked {
			ack.Sequence = append(ack.Sequence, sequence)
		}
		log.Debugf("acknowledging [%d] payloads, [%d] buffered", len(ack.Sequence), len(buffer.buffer))

		if err := buffer.forwarder.ForwardAcknowledgement(buffer.SrcAddress, ack); err != nil {
			return err
		}

		buffer.acked = make(map[int32]int64) // clear
		buffer.lastAck = now

	} else {
		log.Debug("not ready to acknowledge")
	}

	return nil
}

func (buffer *PayloadBuffer) retransmit() error {
	if len(buffer.buffer) > 0 {
		log := pfxlog.ContextLogger(fmt.Sprintf("s/" + buffer.sessionId.Token))

		now := info.NowInMilliseconds()
		var unacked []*Payload
		for _, v := range buffer.buffer {
			if v.payload.GetSequence() < buffer.receivedAckHwm && now-v.age > buffer.config.retransmitAge {
				unacked = append(unacked, v.payload)
				v.age = now
			}
		}
		if len(unacked) > 0 {
			for _, payload := range unacked {
				if err := buffer.forwarder.ForwardPayload(buffer.SrcAddress, payload); err != nil {
					return err
				}
			}
			log.Infof("retransmitted [%d] payloads, [%d] buffered", len(unacked), len(buffer.buffer))

		} else {
			log.Debug("no payloads to retransmit")
		}
	}
	return nil
}

func (buffer *PayloadBuffer) debug(now int64) {
	pfxlog.ContextLogger(buffer.sessionId.Token).Debugf("buffer=[%d], acked=[%d], lastAck=[%d ms.], receivedAckHwm=[%d]",
		len(buffer.buffer), len(buffer.acked), now-buffer.lastAck, buffer.receivedAckHwm)
}
