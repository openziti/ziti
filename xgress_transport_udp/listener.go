/*
	Copyright 2019 NetFoundry, Inc.

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

package xgress_transport_udp

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-fabric/xgress_udp"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport/udp"
	"io"
	"net"
	"time"
)

func (l *listener) Listen(address string, bindHandler xgress.BindHandler) error {
	l.address = address
	l.bindHandler = bindHandler

	pfxlog.Logger().Infof("parsing xgress address: %v", address)
	packetAddress, err := xgress_udp.Parse(address)
	if err != nil {
		return fmt.Errorf("cannot listen on invalid address [%s] (%w)", address, err)
	}

	pfxlog.Logger().Infof("dialing packet address [%v]", packetAddress)
	conn, err := net.ListenPacket(packetAddress.Network(), packetAddress.Address())
	if err != nil {
		return err
	}

	l.conn = conn
	go l.relayIncomingPackets()
	go l.rx()

	return nil
}

func (l *listener) WriteTo(data []byte, addr net.Addr) (n int, err error) {
	return l.conn.WriteTo(data, addr)
}

func (l *listener) GetSession(sessionId string) (xgress_udp.Session, bool) {
	session, found := l.sessions[sessionId]
	return session, found
}

func (l *listener) DeleteSession(sessionId string) {
	delete(l.sessions, sessionId)
}

func (l *listener) QueueEvent(event xgress_udp.EventHandler) {
	l.eventChan <- event
}

func (l *listener) LogContext() string {
	return l.address
}

func (l *listener) close() {
	logger := pfxlog.ContextLogger(l.address)
	if l.conn != nil {
		if err := l.conn.Close(); err != nil {
			logger.Errorf("failure closing packet conn. (%w)", err)
		}
	}
}

func (l *listener) relayIncomingPackets() {
	defer l.close()

	logger := pfxlog.ContextLogger(l.address)

	for {
		buf := make([]byte, udp.MaxPacketSize)
		logger.Debugf("Trying to read next packet")
		bytesRead, addr, err := l.conn.ReadFrom(buf)
		logger.Debugf("Packet read complete: %v bytes read", bytesRead)

		if bytesRead > 0 {
			pd := &packetData{
				buffer: buf[:bytesRead],
				source: addr,
			}
			l.dataChan <- pd
		}

		if err != nil {
			logger := pfxlog.ContextLogger(l.address)
			logger.Error(err)
		}
	}
}

func (l *listener) rx() {
	logger := pfxlog.ContextLogger(l.address)

	sessionScanTimer := time.Tick(time.Second * 10)

	for {
		select {
		case data := <-l.dataChan:
			logger.Debugf("handling data. bytes %v from source %v", len(data.buffer), data.source)
			sessionId := data.source.String()
			session, present := l.sessions[sessionId]
			if !present {
				session = &packetSession{
					listener:             l,
					readC:                make(chan []byte, 10),
					addr:                 data.source,
					state:                xgress_udp.SessionStateNew,
					timeoutIntervalNanos: defaultTimeoutInterval.Nanoseconds(),
				}
				session.MarkActivity()
				l.sessions[sessionId] = session
				logger.Debugf("no session present. authenticating session for %v", data.source)
				go l.handleConnect(data.buffer, session)

			} else if session.State() == xgress_udp.SessionStateEstablished {
				logger.Debugf("session established for %v, forwarding data", data.source)
				session.MarkActivity()
				session.QueueRead(data.buffer)
				logger.Debugf("session established for %v. Data forwarded", data.source)
			}

		case event := <-l.eventChan:
			event.Handle(l)

		case tick := <-sessionScanTimer:
			nowNanos := tick.UnixNano()
			for _, session := range l.sessions {
				if session.TimeoutNanos() < nowNanos {
					_ = session.Close() // always returns nil
				}
			}
		}
	}
}

func (session *packetSession) State() xgress_udp.SessionState {
	return session.state
}

func (session *packetSession) SetState(state xgress_udp.SessionState) {
	session.state = state
}

func (session *packetSession) Address() net.Addr {
	return session.addr
}

func (session *packetSession) ReadPayload() ([]byte, map[uint8][]byte, error) {
	select {
	case buffer, chanOpen := <-session.readC:
		if !chanOpen {
			return buffer, nil, io.EOF
		}
		return buffer, nil, nil
	}
}

func (session *packetSession) Write(p []byte) (n int, err error) {
	session.listener.QueueEvent((*sessionUpdateEvent)(session)) // queues session timeout to be updated
	return session.listener.WriteTo(p, session.addr)
}

func (session *packetSession) WritePayload(p []byte, headers map[uint8][]byte) (n int, err error) {
	return session.Write(p)
}

func (session *packetSession) QueueRead(data []byte) {
	session.readC <- data
}

func (session *packetSession) Close() error {
	session.listener.QueueEvent((*sessionCloseEvent)(session)) // queues session to be removed
	return nil
}

func (session *packetSession) LogContext() string {
	return session.addr.String()
}

func (session *packetSession) TimeoutNanos() int64 {
	return session.timeoutNanos
}

func (session *packetSession) MarkActivity() {
	session.timeoutNanos = time.Now().UnixNano() + session.timeoutIntervalNanos
}

func (session *packetSession) SessionId() string {
	return session.addr.String()
}

type packetSession struct {
	listener             xgress_udp.Listener
	readC                chan []byte
	addr                 net.Addr
	state                xgress_udp.SessionState
	timeoutIntervalNanos int64
	timeoutNanos         int64
	closed               bool
}

func (l *listener) handleConnect(initialRequest []byte, session xgress_udp.Session) {
	log := pfxlog.ContextLogger(session.LogContext())

	var response *xgress.Response

	request, err := xgress.RequestFromJSON(initialRequest)
	if err != nil {
		log.Error(err)
		response = &xgress.Response{Success: false, Message: "invalid request"}
	} else {
		response = xgress.CreateSession(l.ctrl, session, request, l.bindHandler, l.options)
	}

	l.eventChan <- &sessionResponse{addr: session.Address(), response: response}
}

func newListener(id *identity.TokenId, ctrl xgress.CtrlChannel, options *xgress.Options) xgress.XgressListener {
	return &listener{
		id:        id,
		ctrl:      ctrl,
		options:   options,
		dataChan:  make(chan *packetData, 10),
		eventChan: make(chan xgress_udp.EventHandler, 10),
		sessions:  make(map[string]xgress_udp.Session),
	}
}

type listener struct {
	id          *identity.TokenId
	ctrl        xgress.CtrlChannel
	options     *xgress.Options
	address     string
	bindHandler xgress.BindHandler
	conn        net.PacketConn
	dataChan    chan *packetData
	eventChan   chan xgress_udp.EventHandler
	sessions    map[string]xgress_udp.Session
}

func (response *sessionResponse) Handle(listener xgress_udp.Listener) {
	logger := pfxlog.ContextLogger(listener.LogContext())

	sessionId := response.addr.String()
	session, present := listener.GetSession(sessionId)
	respMsg := response.response
	if !present {
		// session timed out or some other unexpected failure
		respMsg = &xgress.Response{Success: false, Message: "timeout"}
		session = &packetSession{listener: listener, addr: response.addr}
		logger.Debugf("Session %v not found for response", sessionId)
	} else if response.response.Success {
		session.SetState(xgress_udp.SessionStateEstablished)
		logger.Debugf("Session %v found for success response. Marking established", sessionId)
	} else {
		logger.Debugf("Session %v found for failure response. Removing session", sessionId)
		_ = session.Close() // always returns nil
	}
	logger.Debugf("Sending response to client for %v", sessionId)
	err := xgress.SendResponse(respMsg, session)
	logger.Debugf("Response to client for %v sent", sessionId)
	if err != nil {
		logger.Errorf("failure sending response (%v)", err)
	}
}

type sessionResponse struct {
	addr     net.Addr
	response *xgress.Response
}

type packetData struct {
	buffer []byte
	source net.Addr
}

type sessionUpdateEvent packetSession

func (session *sessionUpdateEvent) Handle(listener xgress_udp.Listener) {
	(*packetSession)(session).MarkActivity()
}

type sessionCloseEvent packetSession

func (closeEvent *sessionCloseEvent) Handle(listener xgress_udp.Listener) {
	// Note: The Xgress will close when EOF is returned from a read. That will also cause the session to be closed
	//       The session packet will return EOF from a read when the session.readC is closed
	session := (*packetSession)(closeEvent)
	if !session.closed {
		close(session.readC)
		listener.DeleteSession(session.SessionId())
		session.closed = true
	}
}

type sessionState uint8

const (
	sessionStateNew sessionState = iota
	sessionStateEstablished
)

type packetEvent interface {
	handle(listener *listener)
}

const (
	defaultTimeoutInterval = time.Minute
)
