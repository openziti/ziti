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
	"encoding/binary"
	"fmt"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/util/uuidz"
	"github.com/sirupsen/logrus"
	"math"
)

const (
	MinHeaderKey = 2000
	MaxHeaderKey = MinHeaderKey + int32(math.MaxUint8)

	HeaderKeySessionId = 2256
	HeaderKeySequence  = 2257
	HeaderKeyFlags     = 2258

	ContentTypePayloadType         = 1100
	ContentTypeAcknowledgementType = 1101
)

var ContentTypeValue = map[string]int32{
	"PayloadType":         ContentTypePayloadType,
	"AcknowledgementType": ContentTypeAcknowledgementType,
}

type Originator int32

const (
	Initiator  Originator = 0
	Terminator Originator = 1
)

func (o Originator) String() string {
	if o == Initiator {
		return "Initiator"
	}
	return "Terminator"
}

type PayloadFlag uint32

const (
	PayloadFlagSessionEnd   PayloadFlag = 1
	PayloadFlagEgress       PayloadFlag = 2
	PayloadFlagSessionStart PayloadFlag = 4
)

type Header struct {
	SessionId string
	Flags     uint32
}

func (header *Header) GetSessionId() string {
	return header.SessionId
}

func (header *Header) GetFlags() string {
	return header.SessionId
}

func (header *Header) GetOriginator() Originator {
	if isPayloadFlagSet(header.Flags, PayloadFlagEgress) {
		return Terminator
	}
	return Initiator
}

func (header *Header) unmarshallHeader(msg *channel2.Message) error {
	sessionId, ok := msg.Headers[HeaderKeySessionId]
	if !ok {
		return fmt.Errorf("no sessionId found in xgress payload message")
	}

	// If no flags are present, it just means no flags have been set
	flags, _ := msg.GetUint32Header(HeaderKeyFlags)

	header.SessionId = string(sessionId)
	header.Flags = flags

	return nil
}

func (header *Header) marshallHeader(msg *channel2.Message) {
	msg.Headers[HeaderKeySessionId] = []byte(header.SessionId)
	if header.Flags != 0 {
		msg.PutUint32Header(HeaderKeyFlags, header.Flags)
	}
}

func NewAcknowledgement(sessionId string, originator Originator) *Acknowledgement {
	return &Acknowledgement{
		Header: Header{
			SessionId: sessionId,
			Flags:     SetOriginatorFlag(0, originator),
		},
	}
}

type Acknowledgement struct {
	Header
	Sequence []int32
}

func (ack *Acknowledgement) GetSequence() []int32 {
	return ack.Sequence
}

func (ack *Acknowledgement) marshallSequence() []byte {
	if len(ack.Sequence) == 0 {
		return nil
	}
	buf := make([]byte, len(ack.Sequence)*4)
	nextWriteBuf := buf
	for _, seq := range ack.Sequence {
		binary.BigEndian.PutUint32(nextWriteBuf, uint32(seq))
		nextWriteBuf = nextWriteBuf[4:]
	}
	return buf
}

func (ack *Acknowledgement) unmarshallSequence(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	if len(data)%4 != 0 {
		return fmt.Errorf("received sequence with wrong number of bytes: %v", len(data))
	}
	ack.Sequence = make([]int32, len(data)/4)

	nextReadBuf := data
	for i, _ := range ack.Sequence {
		ack.Sequence[i] = int32(binary.BigEndian.Uint32(nextReadBuf))
		nextReadBuf = nextReadBuf[4:]
	}
	return nil
}

func (ack *Acknowledgement) Marshall() *channel2.Message {
	msg := channel2.NewMessage(ContentTypeAcknowledgementType, ack.marshallSequence())
	ack.marshallHeader(msg)
	return msg
}

func UnmarshallAcknowledgement(msg *channel2.Message) (*Acknowledgement, error) {
	ack := &Acknowledgement{}

	if err := ack.unmarshallHeader(msg); err != nil {
		return nil, err
	}
	if err := ack.unmarshallSequence(msg.Body); err != nil {
		return nil, err
	}

	return ack, nil
}

type Payload struct {
	Header
	Sequence int32
	Headers  map[uint8][]byte
	Data     []byte
}

func (payload *Payload) GetSequence() int32 {
	return payload.Sequence
}

func (payload *Payload) Marshall() *channel2.Message {
	msg := channel2.NewMessage(ContentTypePayloadType, payload.Data)
	for key, value := range payload.Headers {
		msgHeaderKey := MinHeaderKey + int32(key)
		msg.Headers[msgHeaderKey] = value
	}
	payload.marshallHeader(msg)
	msg.PutUint64Header(HeaderKeySequence, uint64(payload.Sequence))
	return msg
}

func UnmarshallPayload(msg *channel2.Message) (*Payload, error) {
	var headers map[uint8][]byte
	for key, val := range msg.Headers {
		if key >= MinHeaderKey && key <= MaxHeaderKey {
			if headers == nil {
				headers = make(map[uint8][]byte)
			}
			xgressHeaderKey := uint8(key - MinHeaderKey)
			headers[xgressHeaderKey] = val
		}
	}

	payload := &Payload{
		Headers: headers,
		Data:    msg.Body,
	}

	if err := payload.unmarshallHeader(msg); err != nil {
		return nil, err
	}

	sequence, ok := msg.GetUint64Header(HeaderKeySequence)
	if !ok {
		return nil, fmt.Errorf("no sequence found in xgress payload message")
	}
	payload.Sequence = int32(sequence)

	return payload, nil
}

func isPayloadFlagSet(flags uint32, flag PayloadFlag) bool {
	return PayloadFlag(flags)&flag == flag
}

func (payload *Payload) IsSessionEndFlagSet() bool {
	return isPayloadFlagSet(payload.Flags, PayloadFlagSessionEnd)
}

func (payload *Payload) IsSessionStartFlagSet() bool {
	return isPayloadFlagSet(payload.Flags, PayloadFlagSessionStart)
}

func SetOriginatorFlag(flags uint32, originator Originator) uint32 {
	if originator == Initiator {
		return ^uint32(PayloadFlagEgress) & flags
	}
	return uint32(PayloadFlagEgress) | flags
}

func (payload *Payload) GetLoggerFields() logrus.Fields {
	return logrus.Fields{
		"seq":    payload.Sequence,
		"origin": payload.GetOriginator(),
		"uuid":   uuidz.ToString(payload.Headers[HeaderKeyUUID]),
	}
}
