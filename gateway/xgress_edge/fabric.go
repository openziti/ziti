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
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/util/concurrenz"
	"github.com/netfoundry/ziti-foundation/util/sequencer"
	"github.com/netfoundry/ziti-sdk-golang/ziti/edge"
	"io"
)

type localMessageSink struct {
	edge.MsgChannel
	seq       sequencer.Sequencer
	closeCB   func(connId uint32)
	newSinkCB func(sink *localMessageSink)
	closed    concurrenz.AtomicBoolean
}

type localListener struct {
	localMessageSink
	token   string
	service string
	parent  *ingressProxy
}

func (conn *localMessageSink) newSink(connId uint32, options *Options) *localMessageSink {
	result := &localMessageSink{
		MsgChannel: *edge.NewEdgeMsgChannel(conn.Channel, connId),
		seq:        sequencer.NewSingleWriterSeq(options.MaxOutOfOrderMsgs),
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

	for {
		next := conn.seq.GetNext()
		if next == nil {
			log.Debug("sequencer closed, return EOF")
			conn.closeCB(conn.Id())
			return nil, nil, io.EOF // io.EOF signals xgress to shutdown
		}

		em := next.(*edge.MsgEvent)
		log = log.WithFields(em.GetLoggerFields())
		log.Debug("processing")

		switch em.Msg.ContentType {
		case edge.ContentTypeData:
			log.Debugf("received data message with payload size %v", len(em.Msg.Body))
			return em.Msg.Body, conn.getHeaderMap(em.Msg), nil

		case edge.ContentTypeStateClosed:
			log.Debug("received close message, closing connection and returning EOF")
			conn.close(false, "close message received")
			conn.closeCB(conn.Id())
			return nil, nil, io.EOF // io.EOF signals xgress to shutdown

		default:
			log.Error("unexpected message type")
		}
	}
}

func (conn *localMessageSink) WritePayload(p []byte, headers map[uint8][]byte) (n int, err error) {
	msgUUID := headers[xgress.HeaderKeyUUID]
	return conn.WriteTraced(p, msgUUID)
}

func (conn *localMessageSink) Close() error {
	conn.close(true, "close called")

	return nil
}

func (conn *localMessageSink) close(notify bool, reason string) {
	if !conn.closed.CompareAndSwap(false, true) {
		// already closed
		return
	}

	log := pfxlog.ContextLogger(conn.Channel.Label()).WithField("connId", conn.Id())
	log.Infof("closing message sink, reason: %v", reason)
	if notify {
		// Notify edge client of close
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
	conn.seq.Close()
}

func (conn *localMessageSink) Accept(event *edge.MsgEvent) {
	if err := conn.seq.PutSequenced(event.Seq, event); err != nil {
		pfxlog.Logger().WithFields(event.GetLoggerFields()).Errorf("failed to dispatch to fabric: (%v)", err)
	}
}

func (conn *localMessageSink) getHeaderMap(message *channel2.Message) map[uint8][]byte {
	msgUUID, found := message.Headers[edge.UUIDHeader]
	if found {
		return map[uint8][]byte{xgress.HeaderKeyUUID: msgUUID}
	}
	return nil
}
