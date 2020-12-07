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

package xgress_edge

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/sdk-golang/ziti/edge"
	"io"
)

const (
	PayloadFlagsHeader uint8 = 0x10
)

// headers to pass through fabric to the other side
var headersTofabric = map[int32]uint8{
	edge.FlagsHeader: PayloadFlagsHeader,
}

var headersFromFabric = map[uint8]int32{
	PayloadFlagsHeader: edge.FlagsHeader,
}

type localMessageSink struct {
	edge.MsgChannel
	seq       MsgQueue
	closeCB   func(connId uint32)
	newSinkCB func(sink *localMessageSink)
	closed    concurrenz.AtomicBoolean
}

type localListener struct {
	localMessageSink
	terminatorIdRef *concurrenz.AtomicString
	service         string
	parent          *ingressProxy
}

func (conn *localMessageSink) newSink(connId uint32) *localMessageSink {
	result := &localMessageSink{
		MsgChannel: *edge.NewEdgeMsgChannel(conn.Channel, connId),
		seq:        NewMsgQueue(4),
		closeCB:    conn.closeCB,
		newSinkCB:  conn.newSinkCB,
	}

	// TODO: Evaluate best way to get new conn in map. Split dial? Any way to use regular map?
	if conn.newSinkCB != nil {
		conn.newSinkCB(result)
	}
	return result
}

func (conn *localMessageSink) LogContext() string {
	return conn.Channel.Label()
}

func (conn *localMessageSink) ReadPayload() ([]byte, map[uint8][]byte, error) {
	log := pfxlog.ContextLogger(conn.Channel.Label()).WithField("connId", conn.Id())

	msg := conn.seq.Pop()
	if msg == nil {
		log.Debug("sequencer closed, return EOF")
		conn.closeCB(conn.Id())
		return nil, nil, io.EOF // io.EOF signals xgress to shutdown
	}

	log = log.WithFields(edge.GetLoggerFields(msg))
	log.Debug("processing")

	switch msg.ContentType {
	case edge.ContentTypeData:
		log.Debugf("received data message with payload size %v", len(msg.Body))
		return msg.Body, conn.getHeaderMap(msg), nil

	case edge.ContentTypeStateClosed:
		log.Debug("received close message, closing connection and returning EOF")
		conn.close(false, "close message received")
		conn.closeCB(conn.Id())
		return nil, nil, io.EOF // io.EOF signals xgress to shutdown

	default:
		log.Error("unexpected message type, closing connection")
		conn.close(false, "close message received")
		conn.closeCB(conn.Id())
		return nil, nil, io.EOF // io.EOF signals xgress to shutdown
	}
}

func (conn *localMessageSink) WritePayload(p []byte, headers map[uint8][]byte) (n int, err error) {
	var msgUUID []byte
	var edgeHdrs map[int32][]byte

	if headers != nil {
		msgUUID = headers[xgress.HeaderKeyUUID]

		edgeHdrs = make(map[int32][]byte)
		for k, v := range headers {
			if edgeHeader, found := headersFromFabric[k]; found {
				edgeHdrs[edgeHeader] = v
			}
		}
	}

	msg := edge.NewDataMsg(conn.Id(), conn.NextMsgId(), p)
	if msgUUID != nil {
		msg.Headers[edge.UUIDHeader] = msgUUID
	}

	for k, v := range edgeHdrs {
		msg.Headers[k] = v
	}

	conn.TraceMsg("write", msg)
	pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Tracef("writing %v bytes", len(p))

	if err = conn.Channel.Send(msg); err != nil {
		return 0, err
	}

	return len(p), nil
}

func (conn *localMessageSink) Close() error {
	conn.close(true, "close called")
	return nil
}

func (conn *localMessageSink) HandleMuxClose() error {
	conn.close(false, "channel closed")
	return nil
}

func (conn *localMessageSink) close(notify bool, reason string) {
	if !conn.closed.CompareAndSwap(false, true) {
		// already closed
		return
	}

	log := pfxlog.ContextLogger(conn.Channel.Label()).WithField("connId", conn.Id())
	log.Debugf("closing message sink, reason: %v", reason)
	if notify {
		// Notify edge client of close
		log.Debug("sennding closed to SDK client")
		closeMsg := edge.NewStateClosedMsg(conn.Id(), "")
		if err := conn.SendState(closeMsg); err != nil {
			log.WithError(err).Warn("unable to send close msg to edge client")
		}
	}

	// remove ourselves from mux, etc
	conn.closeCB(conn.Id())

	// When nextSeq is closed, GetNext in Read() will return a nil.
	// This will cause an io.EOF to be returned to the xgress read loop, which will cause that
	// to terminate
	log.Debug("closing channel sequencer, which should cause xgress to close")
	conn.seq.Close()
}

func (conn *localMessageSink) Accept(msg *channel2.Message) {
	if err := conn.seq.Push(msg); err != nil {
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Errorf("failed to dispatch to fabric: (%v)", err)
	}
}

func (conn *localMessageSink) getHeaderMap(message *channel2.Message) map[uint8][]byte {
	headers := make(map[uint8][]byte)
	msgUUID, found := message.Headers[edge.UUIDHeader]
	if found {
		headers[xgress.HeaderKeyUUID] = msgUUID
	}

	for k, v := range message.Headers {
		if pHdr, found := headersTofabric[k]; found {
			headers[pHdr] = v
		}
	}

	return headers
}
