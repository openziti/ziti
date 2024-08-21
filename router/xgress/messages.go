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
	"encoding/binary"
	"fmt"
	"github.com/openziti/channel/v2"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/foundation/v2/uuidz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"math"
)

const (
	MinHeaderKey = 2000
	MaxHeaderKey = MinHeaderKey + int32(math.MaxUint8)

	HeaderKeyCircuitId      = 2256
	HeaderKeySequence       = 2257
	HeaderKeyFlags          = 2258
	HeaderKeyRecvBufferSize = 2259
	HeaderKeyRTT            = 2260

	ContentTypePayloadType         = 1100
	ContentTypeAcknowledgementType = 1101
	ContentTypeControlType         = 1102
)

var ContentTypeValue = map[string]int32{
	"PayloadType":         ContentTypePayloadType,
	"AcknowledgementType": ContentTypeAcknowledgementType,
	"ControlType":         ContentTypeControlType,
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
	PayloadFlagCircuitEnd   PayloadFlag = 1
	PayloadFlagOriginator   PayloadFlag = 2
	PayloadFlagCircuitStart PayloadFlag = 4
	PayloadFlagChunk        PayloadFlag = 8
)

type Header struct {
	CircuitId      string
	Flags          uint32
	RecvBufferSize uint32
	RTT            uint16
}

func (header *Header) GetCircuitId() string {
	return header.CircuitId
}

func (header *Header) GetFlags() uint32 {
	return header.Flags
}

func (header *Header) GetOriginator() Originator {
	if isPayloadFlagSet(header.Flags, PayloadFlagOriginator) {
		return Terminator
	}
	return Initiator
}

func (header *Header) unmarshallHeader(msg *channel.Message) error {
	circuitId, ok := msg.Headers[HeaderKeyCircuitId]
	if !ok {
		return fmt.Errorf("no circuitId found in xgress payload message")
	}

	// If no flags are present, it just means no flags have been set
	flags, _ := msg.GetUint32Header(HeaderKeyFlags)

	header.CircuitId = string(circuitId)
	header.Flags = flags
	if header.RecvBufferSize, ok = msg.GetUint32Header(HeaderKeyRecvBufferSize); !ok {
		header.RecvBufferSize = math.MaxUint32
	}

	header.RTT, _ = msg.GetUint16Header(HeaderKeyRTT)

	return nil
}

func (header *Header) marshallHeader(msg *channel.Message) {
	msg.Headers[HeaderKeyCircuitId] = []byte(header.CircuitId)
	if header.Flags != 0 {
		msg.PutUint32Header(HeaderKeyFlags, header.Flags)
	}

	msg.PutUint32Header(HeaderKeyRecvBufferSize, header.RecvBufferSize)
}

func NewAcknowledgement(circuitId string, originator Originator) *Acknowledgement {
	return &Acknowledgement{
		Header: Header{
			CircuitId: circuitId,
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
	for i := range ack.Sequence {
		ack.Sequence[i] = int32(binary.BigEndian.Uint32(nextReadBuf))
		nextReadBuf = nextReadBuf[4:]
	}
	return nil
}

func (ack *Acknowledgement) Marshall() *channel.Message {
	msg := channel.NewMessage(ContentTypeAcknowledgementType, ack.marshallSequence())
	msg.PutUint16Header(HeaderKeyRTT, ack.RTT)
	ack.marshallHeader(msg)
	return msg
}

func UnmarshallAcknowledgement(msg *channel.Message) (*Acknowledgement, error) {
	ack := &Acknowledgement{}

	if err := ack.unmarshallHeader(msg); err != nil {
		return nil, err
	}
	if err := ack.unmarshallSequence(msg.Body); err != nil {
		return nil, err
	}

	return ack, nil
}

func (ack *Acknowledgement) GetLoggerFields() logrus.Fields {
	return logrus.Fields{
		"circuitId":          ack.CircuitId,
		"linkRecvBufferSize": ack.RecvBufferSize,
		"seq":                fmt.Sprintf("%+v", ack.Sequence),
		"RTT":                ack.RTT,
	}
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

func (payload *Payload) Marshall() *channel.Message {
	msg := channel.NewMessage(ContentTypePayloadType, payload.Data)
	for key, value := range payload.Headers {
		msgHeaderKey := MinHeaderKey + int32(key)
		msg.Headers[msgHeaderKey] = value
	}
	payload.marshallHeader(msg)
	msg.PutUint64Header(HeaderKeySequence, uint64(payload.Sequence))
	msg.PutUint16Header(HeaderKeyRTT, uint16(info.NowInMilliseconds()))

	return msg
}

func UnmarshallPayload(msg *channel.Message) (*Payload, error) {
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

func setPayloadFlag(flags uint32, flag PayloadFlag) uint32 {
	return uint32(PayloadFlag(flags) | flag)
}

func (payload *Payload) IsCircuitEndFlagSet() bool {
	return isPayloadFlagSet(payload.Flags, PayloadFlagCircuitEnd)
}

func (payload *Payload) IsCircuitStartFlagSet() bool {
	return isPayloadFlagSet(payload.Flags, PayloadFlagCircuitStart)
}

func SetOriginatorFlag(flags uint32, originator Originator) uint32 {
	if originator == Initiator {
		return ^uint32(PayloadFlagOriginator) & flags
	}
	return uint32(PayloadFlagOriginator) | flags
}

func (payload *Payload) GetLoggerFields() logrus.Fields {
	result := logrus.Fields{
		"circuitId": payload.CircuitId,
		"seq":       payload.Sequence,
		"origin":    payload.GetOriginator(),
	}

	if uuidVal, found := payload.Headers[HeaderKeyUUID]; found {
		result["uuid"] = uuidz.ToString(uuidVal)
	}

	return result
}

type ControlType byte

func (self ControlType) String() string {
	switch self {
	case ControlTypeTraceRoute:
		return "traceroute"
	case ControlTypeTraceRouteResponse:
		return "traceroute_response"
	default:
		return fmt.Sprintf("unhandled: %v", byte(self))
	}
}

const (
	ControlTypeTraceRoute         ControlType = 1
	ControlTypeTraceRouteResponse ControlType = 2
)

const (
	ControlHopCount  = 20
	ControlHopType   = 21
	ControlHopId     = 22
	ControlTimestamp = 23
	ControlUserVal   = 24
	ControlError     = 25
)

type Control struct {
	Type      ControlType
	CircuitId string
	Headers   channel.Headers
}

func (self *Control) Marshall() *channel.Message {
	msg := channel.NewMessage(ContentTypeControlType, append([]byte{byte(self.Type)}, self.CircuitId...))
	msg.Headers = self.Headers
	return msg
}

func UnmarshallControl(msg *channel.Message) (*Control, error) {
	if len(msg.Body) < 2 {
		return nil, errors.New("control message body too short")
	}
	return &Control{
		Type:      ControlType(msg.Body[0]),
		CircuitId: string(msg.Body[1:]),
		Headers:   msg.Headers,
	}, nil
}

func (self *Control) IsTypeTraceRoute() bool {
	return self.Type == ControlTypeTraceRoute
}

func (self *Control) IsTypeTraceRouteResponse() bool {
	return self.Type == ControlTypeTraceRouteResponse
}

func (self *Control) DecrementAndGetHop() uint32 {
	hop, _ := self.Headers.GetUint32Header(ControlHopCount)
	if hop == 0 {
		return 0
	}
	hop--
	self.Headers.PutUint32Header(ControlHopCount, hop)
	return hop
}

func (self *Control) CreateTraceResponse(hopType, hopId string) *Control {
	resp := &Control{
		Type:      ControlTypeTraceRouteResponse,
		CircuitId: self.CircuitId,
		Headers:   self.Headers,
	}
	resp.Headers.PutStringHeader(ControlHopType, hopType)
	resp.Headers.PutStringHeader(ControlHopId, hopId)
	return resp
}

func (self *Control) GetLoggerFields() logrus.Fields {
	result := logrus.Fields{
		"circuitId": self.CircuitId,
		"type":      self.Type,
	}

	if uuidVal, found := self.Headers[HeaderKeyUUID]; found {
		result["uuid"] = uuidz.ToString(uuidVal)
	}

	return result
}

func RespondToTraceRequest(headers channel.Headers, hopType, hopId string, response ControlReceiver) {
	resp := &Control{Headers: headers}
	resp.DecrementAndGetHop()
	resp.Headers.PutStringHeader(ControlHopType, hopType)
	resp.Headers.PutStringHeader(ControlHopId, hopId)
	response.HandleControlReceive(ControlTypeTraceRouteResponse, headers)
}

type InvalidTerminatorError struct {
	InnerError error
}

func (e InvalidTerminatorError) Error() string {
	return e.InnerError.Error()
}

func (e InvalidTerminatorError) Unwrap() error {
	return e.InnerError
}

type MisconfiguredTerminatorError struct {
	InnerError error
}

func (e MisconfiguredTerminatorError) Error() string {
	return e.InnerError.Error()
}

func (e MisconfiguredTerminatorError) Unwrap() error {
	return e.InnerError
}
