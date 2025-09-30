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

package xgress_edge

import (
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/router/state"
	"github.com/openziti/ziti/router/xgress_common"
	"github.com/pkg/errors"
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
	edgeClientConn      *edgeClientConn
	terminatorId        string
	listenerId          string
	serviceSessionToken *state.ServiceSessionToken
	instance            string
	instanceSecret      []byte
	cost                uint16
	precedence          edge_ctrl_pb.TerminatorPrecedence
	hostData            map[uint32][]byte
	assignIds           bool
	useSdkXgress        bool
	onClose             func()
	v2                  bool
	state               concurrenz.AtomicValue[xgress_common.TerminatorState]
	supportsInspect     bool
	operationActive     atomic.Bool
	createTime          time.Time
	lastAttempt         time.Time
	establishCallback   func(result edge_ctrl_pb.CreateTerminatorResult)
	lock                sync.Mutex
	rateLimitCallback   rate.RateLimitControl
}

func (self *edgeTerminator) getIdentityId() string {
	if self.edgeClientConn == nil {
		return ""
	}
	return self.edgeClientConn.getIdentityId()
}

func (self *edgeTerminator) replace(other *edgeTerminator) {
	other.lock.Lock()
	terminatorId := other.terminatorId
	state := other.state.Load()
	rateLimitCallback := other.rateLimitCallback
	operationActive := other.operationActive.Load()
	createTime := other.createTime
	lastAttempt := other.lastAttempt
	other.lock.Unlock()

	self.lock.Lock()
	self.terminatorId = terminatorId
	self.setState(state, "replacing existing terminator")
	self.rateLimitCallback = rateLimitCallback
	self.operationActive.Store(operationActive)
	self.createTime = createTime
	self.lastAttempt = lastAttempt
	self.lock.Unlock()
}

func (self *edgeTerminator) IsEstablishing() bool {
	return self.state.Load() == xgress_common.TerminatorStateEstablishing
}

func (self *edgeTerminator) IsDeleting() bool {
	return self.state.Load() == xgress_common.TerminatorStateDeleting
}

func (self *edgeTerminator) setState(state xgress_common.TerminatorState, reason string) {
	if oldState := self.state.Load(); oldState != state {
		self.state.Store(state)
		pfxlog.Logger().WithField("terminatorId", self.terminatorId).
			WithField("oldState", oldState).
			WithField("newState", state).
			WithField("reason", reason).
			Info("updated state")
	}
}

func (self *edgeTerminator) updateState(oldState, newState xgress_common.TerminatorState, reason string) bool {
	log := pfxlog.Logger().WithField("terminatorId", self.terminatorId).
		WithField("oldState", oldState).
		WithField("newState", newState).
		WithField("reason", reason)
	success := self.state.CompareAndSwap(oldState, newState)
	if success {
		log.Info("updated state")
	}
	return success
}

func (self *edgeTerminator) inspect(registry *hostedServiceRegistry, fixInvalidTerminators bool, notifyCtrl bool) (*edge.InspectResult, error) {
	result := &edge.InspectResult{
		ConnId: self.MsgChannel.Id(),
		Type:   edge.ConnTypeUnknown,
		Detail: "channel closed",
	}

	var err error
	isInvalid := false
	if !self.GetChannel().IsClosed() {
		msg := channel.NewMessage(edge.ContentTypeConnInspectRequest, nil)
		msg.PutUint32Header(edge.ConnIdHeader, self.Id())
		resp, err := msg.WithTimeout(10 * time.Second).SendForReply(self.GetControlSender())
		if err != nil {
			return nil, fmt.Errorf("unable to check status with sdk client: (%w)", err)
		}

		result, err = edge.UnmarshalInspectResult(resp)
		if err != nil {
			return nil, fmt.Errorf("unable to unmarshal inspect response from sdk client: (%w)", err)
		}

		isInvalid = result != nil && result.Type != edge.ConnTypeBind
	} else {
		isInvalid = true
		err = errors.New("channel closed")
	}

	if isInvalid && fixInvalidTerminators {
		removeResult := registry.handleSdkReturnedInvalid(self, notifyCtrl)
		if removeResult.err != nil {
			return nil, err
		}
		if removeResult.removed || !removeResult.existed {
			return result, err
		}
		current, _ := registry.Get(self.terminatorId)
		if current == nil {
			return result, err
		}

		if current == self { // this shouldn't happen
			return result, errors.New("wasn't able to remove, but is still current")
		}
		return current.inspect(registry, fixInvalidTerminators, notifyCtrl)
	}

	return result, err
}

func (self *edgeTerminator) nextDialConnId() uint32 {
	nextId := atomic.AddUint32(&self.edgeClientConn.idSeq, 1)
	if nextId < math.MaxUint32/2 {
		atomic.StoreUint32(&self.edgeClientConn.idSeq, math.MaxUint32/2)
		nextId = atomic.AddUint32(&self.edgeClientConn.idSeq, 1)
	}
	return nextId
}

func (self *edgeTerminator) close(registry *hostedServiceRegistry, notifySdk bool, notifyCtrl bool, reason string) {
	logger := pfxlog.Logger().
		WithField("terminatorId", self.terminatorId).
		WithField("tokenId", self.serviceSessionToken.TokenId()).
		WithField("reason", reason)

	if notifySdk && !self.GetChannel().IsClosed() {
		// Notify edge client of close
		logger.Debug("sending closed to SDK client")
		closeMsg := edge.NewStateClosedMsg(self.Id(), reason)
		if err := self.SendState(closeMsg); err != nil {
			logger.WithError(err).Warn("unable to send close msg to edge client for hosted service")
		}
	}

	if self.v2 {
		if notifyCtrl {
			registry.queueRemoveTerminatorAsync(self, reason)
		} else {
			self.edgeClientConn.listener.factory.hostedServices.Remove(self, reason)
		}
	} else {
		if notifyCtrl {
			if self.terminatorId != "" {
				logger.Info("removing terminator on controller")

				ctrlCh := self.edgeClientConn.apiSessionToken.SelectCtrlCh(self.edgeClientConn.listener.factory.ctrls)

				if ctrlCh == nil {
					logger.Error("no controller available, unable to remove terminator")
				} else if err := self.edgeClientConn.removeTerminator(ctrlCh, self.serviceSessionToken.TokenId(), self.terminatorId); err != nil {
					logger.WithError(err).Error("failed to remove terminator")
				} else {
					logger.Info("successfully removed terminator")
				}
			} else {
				logger.Warn("edge terminator closing, but no terminator id set, so can't remove on controller")
			}
		}

		logger.Info("terminator removed from router set")
		self.edgeClientConn.listener.factory.hostedServices.Delete(self.serviceSessionToken.TokenId())
	}

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

	if err := mux.Add(result); err != nil {
		return nil, err
	}

	return result, nil
}

func (self *edgeTerminator) SetRateLimitCallback(control rate.RateLimitControl) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.rateLimitCallback = control
}

func (self *edgeTerminator) GetAndClearRateLimitCallback() rate.RateLimitControl {
	self.lock.Lock()
	defer self.lock.Unlock()
	result := self.rateLimitCallback
	self.rateLimitCallback = nil
	return result
}

type edgeXgressConn struct {
	edge.MsgChannel
	mux     edge.ConnMux[*state.ConnState]
	seq     MsgQueue
	onClose func()
	closed  atomic.Bool
	ctrlRx  xgress.ControlReceiver

	data atomic.Pointer[state.ConnState]
}

func (self *edgeXgressConn) GetData() *state.ConnState {
	return self.data.Load()
}

func (self *edgeXgressConn) SetData(data *state.ConnState) {
	self.data.Store(data)
}

func (self *edgeXgressConn) HandleControlMsg(controlType xgress.ControlType, headers channel.Headers, responder xgress.ControlReceiver) error {
	if controlType == xgress.ControlTypeTraceRouteResponse {
		ts, _ := headers.GetUint64Header(xgress.ControlTimestamp)
		hop, _ := headers.GetUint32Header(xgress.ControlHopCount)
		hopType, _ := headers.GetStringHeader(xgress.ControlHopType)
		hopId, _ := headers.GetStringHeader(xgress.ControlHopId)
		requestSeq, _ := headers.GetUint32Header(xgress.ControlUserVal)

		msg := edge.NewTraceRouteResponseMsg(self.Id(), hop, ts, hopType, hopId)
		if traceErr, hasErr := headers.GetStringHeader(xgress.ControlError); hasErr {
			msg.PutStringHeader(edge.TraceError, traceErr)
		}

		msg.PutUint32Header(channel.ReplyForHeader, requestSeq)

		self.TraceMsg("write", msg)
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Trace("writing trace response")

		return self.GetControlSender().Send(msg)
	}

	if controlType == xgress.ControlTypeTraceRoute {
		hop, _ := headers.GetUint32Header(xgress.ControlHopCount)
		if hop == 1 {
			// TODO: find a way to get terminator id for hopId
			xgress.RespondToTraceRequest(channel.Headers(headers), "xgress/edge", "", responder)
			return nil
		}

		ts, _ := headers.GetUint64Header(xgress.ControlTimestamp)
		requestSeq, _ := headers.GetUint32Header(xgress.ControlUserVal)

		msg := edge.NewTraceRouteMsg(self.Id(), hop-1, ts)
		msg.PutUint32Header(edge.TraceSourceRequestIdHeader, requestSeq)

		self.TraceMsg("write", msg)
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Trace("writing trace response")

		return self.GetControlSender().Send(msg)
	}

	return errors.Errorf("unhandled control type: %v", controlType)
}

func (self *edgeXgressConn) LogContext() string {
	return self.GetChannel().Label()
}

func (self *edgeXgressConn) ReadPayload() ([]byte, map[uint8][]byte, error) {
	log := pfxlog.ContextLogger(self.GetChannel().Label()).WithField("connId", self.Id())

	msg := self.seq.Pop()
	if msg == nil {
		log.Debug("sequencer closed, return EOF")
		return nil, nil, xgress.ErrPeerClosed // if the sequencer is closed, the whole conn is being closed
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
		return nil, nil, xgress.ErrPeerClosed // ErrPeerClosed signals the whole xgress to shutdown

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

	msg := edge.NewDataMsg(self.Id(), p)
	if msgUUID != nil {
		msg.Headers[edge.UUIDHeader] = msgUUID
	}

	for k, v := range edgeHdrs {
		msg.Headers[k] = v
	}

	self.TraceMsg("write", msg)
	pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Tracef("writing %v bytes", len(p))

	if err = self.GetDefaultSender().Send(msg); err != nil {
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

	log := pfxlog.ContextLogger(self.GetChannel().Label()).WithField("connId", self.Id())
	log.Debugf("closing edge xgress conn, reason: %v", reason)

	self.mux.Remove(self)

	// When nextSeq is closed, GetNext in Read() will return a nil.
	// This will cause an io.EOF to be returned to the xgress read loop, which will cause that
	// to terminate
	log.Debug("closing channel sequencer, which should cause xgress to close")
	self.seq.Close()

	// we must close the sequencer first, otherwise we can deadlock. The channel rxer can be blocked submitting
	// the sequencer and then notify send will then be stuck writing to a partially closed channel.
	if notify && !self.GetChannel().IsClosed() {
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

		headers.PutUint32Header(xgress.ControlUserVal, uint32(msg.Sequence()))

		self.ctrlRx.HandleControlReceive(xgress.ControlTypeTraceRoute, headers)
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
		headers.PutUint32Header(xgress.ControlUserVal, sourceRequestId)

		self.ctrlRx.HandleControlReceive(xgress.ControlTypeTraceRouteResponse, headers)
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
