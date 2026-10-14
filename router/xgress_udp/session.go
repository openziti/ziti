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

package xgress_udp

import (
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/pkg/errors"
)

func NewPacketSesssion(l Listener, addr net.Addr, timeout int64) Session {
	return &PacketSession{
		listener:             l,
		readC:                make(chan []byte, 10),
		closeNotify:          make(chan struct{}),
		addr:                 addr,
		state:                SessionStateNew,
		timeoutIntervalNanos: timeout,
	}
}

func (s *PacketSession) State() SessionState {
	return s.state
}

func (s *PacketSession) SetState(state SessionState) {
	s.state = state
}

func (s *PacketSession) Address() net.Addr {
	return s.addr
}

func (s *PacketSession) ReadPayload() ([]byte, map[uint8][]byte, error) {
	buffer, chanOpen := <-s.readC
	if !chanOpen {
		return buffer, nil, io.EOF
	}
	return buffer, nil, nil
}

// Write marks the session active and sends p to the session's address. Safe to call from the
// listener's event loop.
func (s *PacketSession) Write(p []byte) (n int, err error) {
	s.MarkActivity()
	return s.listener.WriteTo(p, s.addr)
}

func (s *PacketSession) WritePayload(p []byte, _ map[uint8][]byte) (n int, err error) {
	return s.Write(p)
}

func (s *PacketSession) HandleControlMsg(controlType xgress.ControlType, headers channel.Headers, responder xgress.ControlReceiver) error {
	if controlType == xgress.ControlTypeTraceRoute {
		xgress.RespondToTraceRequest(headers, "xgress/udp", "", responder)
		return nil
	}
	return errors.Errorf("unhandled control type: %v", controlType)
}

// QueueRead hands data to the session's reader, waiting while the read buffer is full. Once the
// session is closed it drops data that does not fit instead, since its reader may already have
// stopped.
func (s *PacketSession) QueueRead(data []byte) {
	select {
	case s.readC <- data:
		return
	default:
	}

	select {
	case s.readC <- data:
	case <-s.closeNotify:
	}
}

// Close releases any QueueRead waiting on the session, then queues the session's removal on the
// listener's event loop, blocking while the loop's queue is full. The event loop must not call it;
// it uses CloseFromEventLoop. Repeated calls are harmless.
func (s *PacketSession) Close() error {
	if s.closing.CompareAndSwap(false, true) {
		close(s.closeNotify)
		s.listener.QueueEvent((*sessionCloseEvent)(s))
	}
	return nil
}

func (s *PacketSession) CloseFromEventLoop() {
	if s.closing.CompareAndSwap(false, true) {
		close(s.closeNotify)
	}
	s.remove(s.listener)
}

func (s *PacketSession) remove(l Listener) {
	if !s.removed {
		close(s.readC)
		l.DeleteSession(s.SessionId())
		s.removed = true
	}
}

func (s *PacketSession) LogContext() string {
	return s.addr.String()
}

func (s *PacketSession) TimeoutNanos() int64 {
	return s.timeoutNanos.Load()
}

func (s *PacketSession) MarkActivity() {
	s.timeoutNanos.Store(time.Now().UnixNano() + s.timeoutIntervalNanos)
}

func (s *PacketSession) SessionId() string {
	return s.addr.String()
}

type PacketSession struct {
	listener             Listener
	readC                chan []byte
	closeNotify          chan struct{}
	closing              atomic.Bool
	addr                 net.Addr
	state                SessionState
	timeoutIntervalNanos int64
	timeoutNanos         atomic.Int64
	removed              bool
}

func (e *sessionCloseEvent) Handle(l Listener) {
	(*PacketSession)(e).remove(l)
}

type sessionCloseEvent PacketSession
