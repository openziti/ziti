/*
	Copyright 2019 Netfoundry, Inc.

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

package edge

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/util/uuidz"
	"encoding/binary"
	"fmt"
	"github.com/sirupsen/logrus"
)

const (
	ContentTypeConnect           = 60783
	ContentTypeStateConnected    = 60784
	ContentTypeStateClosed       = 60785
	ContentTypeData              = 60786
	ContentTypeDial              = 60787
	ContentTypeDialSuccess       = 60788
	ContentTypeDialFailed        = 60789
	ContentTypeBind              = 60790
	ContentTypeUnbind            = 60791
	ContentTypeStateSessionEnded = 60792

	ConnIdHeader       = 1000
	SeqHeader          = 1001
	SessionTokenHeader = 1002

	// Put this in the reflected range so replies will share the same UUID
	UUIDHeader = 128
)

var ContentTypeValue = map[string]int32{
	"EdgeConnectType":        ContentTypeConnect,
	"EdgeStateConnectedType": ContentTypeStateConnected,
	"EdgeStateClosedType":    ContentTypeStateClosed,
	"EdgeDataType":           ContentTypeData,
	"EdgeDialType":           ContentTypeDial,
	"EdgeDialSuccessType":    ContentTypeDialSuccess,
	"EdgeDialFailedType":     ContentTypeDialFailed,
	"EdgeBindType":           ContentTypeBind,
	"EdgeUnbindType":         ContentTypeUnbind,
}

var ContentTypeNames = map[int32]string{
	ContentTypeConnect:        "EdgeConnectType",
	ContentTypeStateConnected: "EdgeStateConnectedType",
	ContentTypeStateClosed:    "EdgeStateClosedType",
	ContentTypeData:           "EdgeDataType",
	ContentTypeDial:           "EdgeDialType",
	ContentTypeDialSuccess:    "EdgeDialSuccessType",
	ContentTypeDialFailed:     "EdgeDialFailedType",
	ContentTypeBind:           "EdgeBindType",
	ContentTypeUnbind:         "EdgeUnbindType",
}

type Sequenced interface {
	GetSequence() uint32
}

type MsgEvent struct {
	ConnId  uint32
	Seq     uint32
	MsgUUID []byte
	Msg     *channel2.Message
}

func (event *MsgEvent) GetSequence() uint32 {
	return event.Seq
}

func UnmarshalMsgEvent(msg *channel2.Message) (*MsgEvent, error) {
	connId, found := msg.GetUint32Header(ConnIdHeader)
	if !found {
		return nil, fmt.Errorf("received edge message with no connId header. content type: %v", msg.ContentType)
	}
	seq, _ := msg.GetUint32Header(SeqHeader)

	event := &MsgEvent{
		ConnId:  connId,
		Seq:     seq,
		MsgUUID: msg.Headers[UUIDHeader],
		Msg:     msg,
	}

	return event, nil
}

func (event *MsgEvent) GetLoggerFields() logrus.Fields {
	msgUUID := uuidz.ToString(event.MsgUUID)
	connId, _ := event.Msg.GetUint32Header(ConnIdHeader)
	seq, _ := event.Msg.GetUint32Header(SeqHeader)

	fields := logrus.Fields{
		"connId":  connId,
		"type":    ContentTypeNames[event.Msg.ContentType],
		"chSeq":   event.Msg.Sequence(),
		"edgeSeq": seq,
	}

	if msgUUID != "" {
		fields["uuid"] = msgUUID
	}
	return fields
}

func SequencerF(v interface{}) uint {
	event := v.(Sequenced)
	return uint(event.GetSequence())
}

func newMsg(contentType int32, connId uint32, seq uint32, data []byte) *channel2.Message {
	msg := channel2.NewMessage(contentType, data)
	msg.PutUint32Header(ConnIdHeader, connId)
	msg.PutUint32Header(SeqHeader, seq)
	return msg
}

func NewDataMsg(connId uint32, seq uint32, data []byte) *channel2.Message {
	return newMsg(ContentTypeData, connId, seq, data)
}

func NewConnectMsg(connId uint32, token string) *channel2.Message {
	return newMsg(ContentTypeConnect, connId, 0, []byte(token))
}

func NewStateConnectedMsg(connId uint32) *channel2.Message {
	return newMsg(ContentTypeStateConnected, connId, 0, nil)
}

func NewStateClosedMsg(connId uint32, message string) *channel2.Message {
	return newMsg(ContentTypeStateClosed, connId, 0, []byte(message))
}

func NewDialMsg(connId uint32, token string) *channel2.Message {
	return newMsg(ContentTypeDial, connId, 0, []byte(token))
}

func NewBindMsg(connId uint32, token string) *channel2.Message {
	return newMsg(ContentTypeBind, connId, 0, []byte(token))
}

func NewUnbindMsg(connId uint32, token string) *channel2.Message {
	return newMsg(ContentTypeUnbind, connId, 0, []byte(token))
}

func NewDialSuccessMsg(connId uint32, newConnId uint32) *channel2.Message {
	newConnIdBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(newConnIdBytes, newConnId)
	msg := newMsg(ContentTypeDialSuccess, connId, 0, newConnIdBytes)
	return msg
}

func NewDialFailedMsg(connId uint32, message string) *channel2.Message {
	return newMsg(ContentTypeDialFailed, connId, 0, []byte(message))
}

func NewStateSessionEndedMsg(reason string) *channel2.Message {
	return newMsg(ContentTypeStateSessionEnded, 0, 0, []byte(reason))
}

type DialResult struct {
	ConnId    uint32
	NewConnId uint32
	Success   bool
	Message   string
}

func UnmarshalDialResult(msg *channel2.Message) (*DialResult, error) {
	connId, found := msg.GetUint32Header(ConnIdHeader)
	if !found {
		return nil, fmt.Errorf("received edge message with no connection id header")
	}

	if msg.ContentType == ContentTypeDialSuccess {
		if len(msg.Body) != 4 {
			return nil, fmt.Errorf("dial success msg improperly formated. body len: %v", len(msg.Body))
		}
		newConnId := binary.LittleEndian.Uint32(msg.Body)
		return &DialResult{
			ConnId:    connId,
			NewConnId: newConnId,
			Success:   true,
		}, nil
	}

	if msg.ContentType == ContentTypeDialFailed {
		return &DialResult{
			ConnId:  connId,
			Success: false,
			Message: string(msg.Body),
		}, nil
	}

	return nil, fmt.Errorf("unexpected response. received %v instead of dial result message", msg.ContentType)
}

func GetLoggerFields(msg *channel2.Message) logrus.Fields {
	msgUUID := uuidz.ToString(msg.Headers[UUIDHeader])

	connId, _ := msg.GetUint32Header(ConnIdHeader)
	seq, _ := msg.GetUint32Header(SeqHeader)

	fields := logrus.Fields{
		"connId":  connId,
		"type":    ContentTypeNames[msg.ContentType],
		"chSeq":   msg.Sequence(),
		"edgeSeq": seq,
	}

	if msgUUID != "" {
		fields["uuid"] = msgUUID
	}

	return fields
}
