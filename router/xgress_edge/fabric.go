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
	"github.com/openziti/channel"
	"github.com/openziti/edge/router/xgress_common"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
	"io"
	"math"
	"sync/atomic"
)

// headers to pass through fabric to the other side
var headersTofabric = map[int32]uint8{
	edge.FlagsHeader: xgress_common.PayloadFlagsHeader,
}

var headersFromFabric = map[uint8]int32{
	xgress_common.PayloadFlagsHeader: edge.FlagsHeader,
}

type edgeTerminator struct {
	edge.MsgChannel
	edgeClientConn *edgeClientConn
	token          string
	terminatorId   string
	assignIds      bool
	onClose        func()
}

func (self *edgeTerminator) nextDialConnId() uint32 {
	nextId := atomic.AddUint32(&self.edgeClientConn.idSeq, 1)
	if nextId < math.MaxUint32/2 {
		atomic.StoreUint32(&self.edgeClientConn.idSeq, math.MaxUint32/2)
		nextId = atomic.AddUint32(&self.edgeClientConn.idSeq, 1)
	}
	return nextId
}

func (self *edgeTerminator) close(notify bool, reason string) {
	logger := pfxlog.Logger().
		WithField("terminatorId", self.terminatorId).
		WithField("token", self.token)

	if notify && !self.IsClosed() {
		// Notify edge client of close
		logger.Debug("sending closed to SDK client")
		closeMsg := edge.NewStateClosedMsg(self.Id(), reason)
		if err := self.SendState(closeMsg); err != nil {
			logger.WithError(err).Warn("unable to send close msg to edge client for hosted service")
		}
	}

	if self.terminatorId != "" {
		logger.Info("removing terminator on controller")
		if err := self.edgeClientConn.removeTerminator(self); err != nil {
			logger.WithError(err).Error("failed to remove terminator")
		}
	} else {
		logger.Warn("edge terminator closing, but no terminator id set, so can't remove on controller")
	}

	logger.Debug("removing terminator on router")
	self.edgeClientConn.listener.factory.hostedServices.Delete(self.token)

	if self.onClose != nil {
		self.onClose()
	}
}

func (self *edgeTerminator) newConnection(connId uint32) (*edgeXgressConn, error) {
	mux := self.edgeClientConn.msgMux
	result := &edgeXgressConn{
		mux:        mux,
		MsgChannel: *edge.NewEdgeMsgChannel(self.edgeClientConn.ch, connId),
		seq:        NewMsgQueue(4),
	}

	if err := mux.AddMsgSink(result); err != nil {
		return nil, err
	}

	return result, nil
}

type edgeXgressConn struct {
	edge.MsgChannel
	mux     edge.MsgMux
	seq     MsgQueue
	onClose func()
	closed  concurrenz.AtomicBoolean
	ctrlRx  xgress.ControlReceiver
}

func (self *edgeXgressConn) HandleControlMsg(controlType xgress.ControlType, headers channel.Headers, responder xgress.ControlReceiver) error {
	if controlType == xgress.ControlTypeTraceRouteResponse {
		ts, _ := headers.GetUint64Header(xgress.ControlTimestamp)
		hop, _ := headers.GetUint32Header(xgress.ControlHopCount)
		hopType, _ := headers.GetStringHeader(xgress.ControlHopType)
		hopId, _ := headers.GetStringHeader(xgress.ControlHopId)
		requestSeq, _ := headers.GetUint32Header(xgress.ControlCustom1)

		msg := edge.NewTraceRouteResponseMsg(self.Id(), hop, ts, hopType, hopId)
		msg.PutUint32Header(channel.ReplyForHeader, requestSeq)

		self.TraceMsg("write", msg)
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Trace("writing trace response")

		return self.Channel.Send(msg)
	}

	if controlType == xgress.ControlTypeTraceRoute {
		hop, _ := headers.GetUint32Header(xgress.ControlHopCount)
		if hop == 1 {
			// TODO: find a way to get terminator id for hopId
			xgress.RespondToTraceRequest(channel.Headers(headers), "xgress/edge", "", responder)
			return nil
		}

		ts, _ := headers.GetUint64Header(xgress.ControlTimestamp)
		requestSeq, _ := headers.GetUint32Header(xgress.ControlCustom1)

		msg := edge.NewTraceRouteMsg(self.Id(), hop-1, ts)
		msg.PutUint32Header(edge.TraceSourceRequestIdHeader, requestSeq)

		self.TraceMsg("write", msg)
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Trace("writing trace response")

		return self.Channel.Send(msg)
	}

	return errors.Errorf("unhandled control type: %v", controlType)
}

func (self *edgeXgressConn) LogContext() string {
	return self.Channel.Label()
}

func (self *edgeXgressConn) ReadPayload() ([]byte, map[uint8][]byte, error) {
	log := pfxlog.ContextLogger(self.Channel.Label()).WithField("connId", self.Id())

	msg := self.seq.Pop()
	if msg == nil {
		log.Debug("sequencer closed, return EOF")
		return nil, nil, io.EOF // io.EOF signals xgress to shutdown
	}

	log = log.WithFields(edge.GetLoggerFields(msg))
	log.Debug("processing")

	switch msg.ContentType {
	case edge.ContentTypeData:
		log.Debugf("received data message with payload size %v", len(msg.Body))
		return msg.Body, self.getHeaderMap(msg), nil

	case edge.ContentTypeStateClosed:
		log.Debug("received close message, closing connection and returning EOF")
		self.close(false, "close message received")
		return nil, nil, io.EOF // io.EOF signals xgress to shutdown

	default:
		log.Error("unexpected message type, closing connection")
		self.close(false, "close message received")
		return nil, nil, io.EOF // io.EOF signals xgress to shutdown
	}
}

func (self *edgeXgressConn) WritePayload(p []byte, headers map[uint8][]byte) (n int, err error) {
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

	msg := edge.NewDataMsg(self.Id(), self.NextMsgId(), p)
	if msgUUID != nil {
		msg.Headers[edge.UUIDHeader] = msgUUID
	}

	for k, v := range edgeHdrs {
		msg.Headers[k] = v
	}

	self.TraceMsg("write", msg)
	pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Tracef("writing %v bytes", len(p))

	if err = self.Channel.Send(msg); err != nil {
		return 0, err
	}

	return len(p), nil
}

func (self *edgeXgressConn) Close() error {
	self.close(true, "close called")
	return nil
}

func (self *edgeXgressConn) HandleMuxClose() error {
	self.close(false, "channel closed")
	return nil
}

func (self *edgeXgressConn) close(notify bool, reason string) {
	if !self.closed.CompareAndSwap(false, true) {
		// already closed
		return
	}

	log := pfxlog.ContextLogger(self.Channel.Label()).WithField("connId", self.Id())
	log.Debugf("closing edge xgress conn, reason: %v", reason)

	self.mux.RemoveMsgSink(self)

	// When nextSeq is closed, GetNext in Read() will return a nil.
	// This will cause an io.EOF to be returned to the xgress read loop, which will cause that
	// to terminate
	log.Debug("closing channel sequencer, which should cause xgress to close")
	self.seq.Close()

	// we must close the sequencer first, otherwise we can deadlock. The channel rxer can be blocked submitting
	// the sequencer and then notify send will then be stuck writing to a partially closed channel.
	if notify && !self.IsClosed() {
		// Notify edge client of close
		log.Debug("sending closed to SDK client")
		closeMsg := edge.NewStateClosedMsg(self.Id(), reason)
		if err := self.SendState(closeMsg); err != nil {
			log.WithError(err).Warn("unable to send close msg to edge client")
		}
	}

	if self.onClose != nil {
		self.onClose()
	}
}

func (self *edgeXgressConn) Accept(msg *channel.Message) {
	if msg.ContentType == edge.ContentTypeTraceRoute {
		headers := channel.Headers{}
		ts, _ := msg.GetUint64Header(edge.TimestampHeader)
		hops, _ := msg.GetUint32Header(edge.TraceHopCountHeader)

		headers.PutUint64Header(xgress.ControlTimestamp, ts)
		headers.PutUint32Header(xgress.ControlHopCount, hops)

		headers.PutUint32Header(xgress.ControlCustom1, uint32(msg.Sequence()))

		self.ctrlRx.HandleControlReceive(xgress.ControlTypeTraceRoute, channel.Headers(headers))
	} else if msg.ContentType == edge.ContentTypeTraceRouteResponse {
		headers := channel.Headers{}
		ts, _ := msg.GetUint64Header(edge.TimestampHeader)
		hopCount, _ := msg.GetUint32Header(edge.TraceHopCountHeader)
		hopType, _ := msg.GetStringHeader(edge.TraceHopTypeHeader)
		hopId, _ := msg.GetStringHeader(edge.TraceHopIdHeader)
		sourceRequestId, _ := msg.GetUint32Header(edge.TraceSourceRequestIdHeader)

		headers.PutUint64Header(xgress.ControlTimestamp, ts)
		headers.PutUint32Header(xgress.ControlHopCount, hopCount)
		headers.PutStringHeader(xgress.ControlHopType, hopType)
		headers.PutStringHeader(xgress.ControlHopId, hopId)
		headers.PutUint32Header(xgress.ControlCustom1, sourceRequestId)

		self.ctrlRx.HandleControlReceive(xgress.ControlTypeTraceRouteResponse, channel.Headers(headers))
	} else {
		if err := self.seq.Push(msg); err != nil {
			pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Errorf("failed to dispatch to fabric: (%v)", err)
		}
	}
}

func (self *edgeXgressConn) getHeaderMap(message *channel.Message) map[uint8][]byte {
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
