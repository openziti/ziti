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

package xgress_udp

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport/udp"
	"io"
	"net"
	"time"
)

const (
	defaultTimeoutInterval = time.Minute
)

type listener struct {
	id          *identity.TokenId
	ctrl        xgress.CtrlChannel
	options     *xgress.Options
	address     string
	bindHandler xgress.BindHandler
	conn        net.PacketConn
	dataChan    chan *packetData
	eventChan   chan packetEvent
	sessions    map[string]*packetSession
}

func newListener(id *identity.TokenId, ctrl xgress.CtrlChannel, options *xgress.Options) *listener {
	return &listener{
		id:        id,
		ctrl:      ctrl,
		options:   options,
		dataChan:  make(chan *packetData, 10),
		eventChan: make(chan packetEvent, 10),
		sessions:  make(map[string]*packetSession),
	}
}

func (listener *listener) Listen(address string, bindHandler xgress.BindHandler) error {
	listener.address = address
	listener.bindHandler = bindHandler

	pfxlog.Logger().Infof("Parsing xgress address: %v", address)
	packetAddress, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("cannot listen on invalid address [%s] (%s)", address, err)
	}

	pfxlog.Logger().Infof("Dialing packet address %v", packetAddress)
	conn, err := net.ListenPacket(packetAddress.Network(), packetAddress.Address())
	if err != nil {
		return err
	}

	listener.conn = conn
	go listener.relayIncomingPackets()
	go listener.rx()

	return nil
}

func (listener *listener) close() {
	logger := pfxlog.ContextLogger(listener.address)
	if listener.conn != nil {
		if err := listener.conn.Close(); err != nil {
			logger.Errorf("failure closing packet conn. (%v)", err)
		}
	}
}

func (listener *listener) relayIncomingPackets() {
	defer listener.close()

	logger := pfxlog.ContextLogger(listener.address)

	for {
		buf := make([]byte, udp.MaxPacketSize)
		logger.Debugf("Trying to read next packet")
		bytesRead, addr, err := listener.conn.ReadFrom(buf)
		logger.Debugf("Packet read complete: %v bytes read", bytesRead)

		if bytesRead > 0 {
			pd := &packetData{
				buffer: buf[:bytesRead],
				source: addr,
			}
			listener.dataChan <- pd
		}

		if err != nil {
			logger := pfxlog.ContextLogger(listener.address)
			logger.Error(err)
		}
	}
}

func (listener *listener) rx() {
	logger := pfxlog.ContextLogger(listener.address)

	sessionScanTimer := time.Tick(time.Second * 10)

	for {
		select {
		case data := <-listener.dataChan:
			logger.Debugf("Handling data. Bytes %v from source %v", len(data.buffer), data.source)
			sessionId := data.source.String()
			session, present := listener.sessions[sessionId]
			if !present {
				session = &packetSession{
					listener:             listener,
					readC:                make(chan []byte, 10),
					addr:                 data.source,
					sessionState:         sessionStateNew,
					timeoutIntervalNanos: defaultTimeoutInterval.Nanoseconds(),
				}
				session.MarkActivity()
				listener.sessions[sessionId] = session
				logger.Debugf("No session present. Authenticating session for %v", data.source)
				go listener.handleConnect(data.buffer, session)
			} else if session.sessionState == sessionStateEstablished {
				logger.Debugf("Session established for %v, forwarding data", data.source)
				session.MarkActivity()
				session.readC <- data.buffer
				logger.Debugf("Session established for %v. Data forwarded", data.source)
			}
		case event := <-listener.eventChan:
			event.handle(listener)
		case tick := <-sessionScanTimer:
			nowNanos := tick.UnixNano()
			for _, session := range listener.sessions {
				if session.timeoutNanos < nowNanos {
					_ = session.Close() // always returns nil
				}
			}
		}
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

type packetSession struct {
	listener             *listener
	readC                chan []byte
	addr                 net.Addr
	sessionState         sessionState
	timeoutIntervalNanos int64
	timeoutNanos         int64
	closed               bool
}

func (session *packetSession) getSessionId() string {
	return session.addr.String()
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
	session.listener.eventChan <- (*sessionUpdateEvent)(session) // queues session timeout to be updated
	return session.listener.conn.WriteTo(p, session.addr)
}

func (session *packetSession) WritePayload(p []byte, headers map[uint8][]byte) (n int, err error) {
	return session.Write(p)
}

func (session *packetSession) Close() error {
	session.listener.eventChan <- (*sessionCloseEvent)(session) // queues session to be removed
	return nil
}

func (session *packetSession) LogContext() string {
	return session.addr.String()
}

func (session *packetSession) MarkActivity() {
	session.timeoutNanos = time.Now().UnixNano() + session.timeoutIntervalNanos
}

func (listener *listener) handleConnect(initialRequest []byte, session *packetSession) {
	log := pfxlog.ContextLogger(session.addr.String())

	var response *xgress.Response

	request, err := xgress.RequestFromJSON(initialRequest)
	if err != nil {
		log.Error(err)
		response = &xgress.Response{Success: false, Message: "invalid request"}
	} else {
		response = xgress.CreateSession(listener.ctrl, session, request, listener.bindHandler, listener.options)
	}

	listener.eventChan <- &sessionResponse{addr: session.addr, response: response}
}

type sessionUpdateEvent packetSession

func (session *sessionUpdateEvent) handle(listener *listener) {
	(*packetSession)(session).MarkActivity()
}

type sessionCloseEvent packetSession

func (closeEvent *sessionCloseEvent) handle(listener *listener) {
	// Note: The Xgress will close when EOF is returned from a read. That will also cause the session to be closed
	//       The session packet will return EOF from a read when the session.readC is closed
	session := (*packetSession)(closeEvent)
	if !session.closed {
		close(session.readC)
		delete(listener.sessions, session.getSessionId())
		session.closed = true
	}
}

type packetData struct {
	buffer []byte
	source net.Addr
}

type sessionResponse struct {
	addr     net.Addr
	response *xgress.Response
}

func (response *sessionResponse) handle(listener *listener) {
	logger := pfxlog.ContextLogger(listener.address)

	sessionId := response.addr.String()
	session, present := listener.sessions[sessionId]
	respMsg := response.response
	if !present {
		// session timed out or some other unexpected failure
		respMsg = &xgress.Response{Success: false, Message: "timeout"}
		session = &packetSession{listener: listener, addr: response.addr}
		logger.Debugf("Session %v not found for response", sessionId)
	} else if response.response.Success {
		session.sessionState = sessionStateEstablished
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
