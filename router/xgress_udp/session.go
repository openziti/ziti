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
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/router/xgress"
	"github.com/pkg/errors"
	"io"
	"net"
	"time"
)

func NewPacketSesssion(l Listener, addr net.Addr, timeout int64) Session {
	return &PacketSession{
		listener:             l,
		readC:                make(chan []byte, 10),
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

func (s *PacketSession) Write(p []byte) (n int, err error) {
	s.listener.QueueEvent((*SessionUpdateEvent)(s))
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

func (s *PacketSession) QueueRead(data []byte) {
	s.readC <- data
}

func (s *PacketSession) Close() error {
	s.listener.QueueEvent((*sessionCloseEvent)(s))
	return nil
}

func (s *PacketSession) LogContext() string {
	return s.addr.String()
}

func (s *PacketSession) TimeoutNanos() int64 {
	return s.timeoutNanos
}

func (s *PacketSession) MarkActivity() {
	s.timeoutNanos = time.Now().UnixNano() + s.timeoutIntervalNanos
}

func (s *PacketSession) SessionId() string {
	return s.addr.String()
}

type PacketSession struct {
	listener             Listener
	readC                chan []byte
	addr                 net.Addr
	state                SessionState
	timeoutIntervalNanos int64
	timeoutNanos         int64
	closed               bool
}

func (s *SessionUpdateEvent) Handle(_ Listener) {
	(*PacketSession)(s).MarkActivity()
}

type SessionUpdateEvent PacketSession

func (e *sessionCloseEvent) Handle(l Listener) {
	session := (*PacketSession)(e)
	if !session.closed {
		close(session.readC)
		l.DeleteSession(session.SessionId())
		session.closed = true
	}
}

type sessionCloseEvent PacketSession
