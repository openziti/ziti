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

package xgress_transport_udp

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xgress_udp"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/info"
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

func (l *listener) Close() error {
	if l.conn != nil {
		return l.conn.Close()
	}
	return nil
}

func (l *listener) relayIncomingPackets() {
	logger := pfxlog.ContextLogger(l.address)

	defer func() {
		if err := l.Close(); err != nil {
			logger.Errorf("failure closing packet conn. (%v)", err)
		}
	}()

	for {
		buf := make([]byte, info.MaxPacketSize)
		logger.Debugf("Trying to read next packet")
		bytesRead, addr, err := l.conn.ReadFrom(buf)
		logger.Debugf("Packet read complete: %v bytes read", bytesRead)

		if bytesRead > 0 {
			l.dataChan <- xgress_udp.NewPacketData(buf[:bytesRead], addr)
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
			logger.Debugf("handling data. bytes [%v] from source [%v]", len(data.Buffer), data.Source)
			sessionId := data.Source.String()
			session, present := l.sessions[sessionId]
			if !present {
				session = xgress_udp.NewPacketSesssion(l, data.Source, time.Minute.Nanoseconds())
				session.MarkActivity()
				l.sessions[sessionId] = session
				logger.Debugf("no session present. authenticating session for %v", data.Source)
				go l.handleConnect(data.Buffer, session)

			} else if session.State() == xgress_udp.SessionStateEstablished {
				logger.Debugf("session established for %v, forwarding data", data.Source)
				session.MarkActivity()
				session.QueueRead(data.Buffer)
				logger.Debugf("session established for %v. Data forwarded", data.Source)
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

func newListener(id *identity.TokenId, ctrl xgress.CtrlChannel, options *xgress.Options) xgress.Listener {
	return &listener{
		id:        id,
		ctrl:      ctrl,
		options:   options,
		dataChan:  make(chan *xgress_udp.PacketData, 10),
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
	dataChan    chan *xgress_udp.PacketData
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
		session = xgress_udp.NewPacketSesssion(listener, response.addr, time.Minute.Nanoseconds())
		logger.Debugf("session [%v] not found for response", sessionId)

	} else if response.response.Success {
		session.SetState(xgress_udp.SessionStateEstablished)
		logger.Debugf("session [%v] found for success response. marked established", sessionId)

	} else {
		logger.Debugf("session [%v] found for failure response. removing session", sessionId)
		_ = session.Close() // always returns nil
	}

	logger.Debugf("sending response to client for [%v]", sessionId)
	err := xgress.SendResponse(respMsg, session)

	if err != nil {
		logger.Errorf("failure sending response (%v)", err)
	}
}

type sessionResponse struct {
	addr     net.Addr
	response *xgress.Response
}
