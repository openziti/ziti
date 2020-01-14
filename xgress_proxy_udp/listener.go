/*
	Copyright 2020 NetFoundry, Inc.

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

package xgress_proxy_udp

import (
	"fmt"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/transport/udp"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"time"
)

func (l *listener) Listen(address string, bindHandler xgress.BindHandler) error {
	l.address = address
	l.bindHandler = bindHandler

	packetAddress, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("error parsing address [%s] (%w)", address, err)
	}

	conn, err := net.ListenPacket(packetAddress.Network(), packetAddress.Address())
	if err != nil {
		return fmt.Errorf("error listening for packets (%w)", err)
	}

	l.conn = conn

	go l.relay()
	go l.rx()

	return nil
}

func (l *listener) relay() {
	defer l.close()

	for {
		buf := make([]byte, udp.MaxPacketSize)
		read, addr, err := l.conn.ReadFrom(buf)
		if err != nil {
			logrus.Errorf("error reading packet (%w)", err)
		}

		if read > 0 {
			pd := &packetData{
				buffer: buf[:read],
				source: addr,
			}
			l.dataChan <- pd
		}
	}
}

func (l *listener) rx() {
	scanTimer := time.Tick(time.Second * 10)

	for {
		select {
		case data := <-l.dataChan:
			sessionId := data.source.String()
			session, found := l.sessions[sessionId]
			if !found {
				session = &packetSession{
					listener:             l,
					readC:                make(chan []byte, 10),
					addr:                 data.source,
					sessionState:         sessionStateNew,
					timeoutIntervalNanos: time.Minute.Nanoseconds(),
				}
				session.MarkActivity()
				l.sessions[sessionId] = session

				go l.handleConnect(session)

				if !session.closed {
					session.readC <- data.buffer
				}

			} else if session.sessionState == sessionStateEstablished {
				session.MarkActivity()
				session.readC <- data.buffer
			}

		case event := <-l.eventChan:
			event.handle(l)

		case tick := <-scanTimer:
			now := tick.UnixNano()
			for _, session := range l.sessions {
				if session.timeoutNanos < now {
					_ = session.Close()
				}
			}
		}
	}
}

func (l *listener) handleConnect(session *packetSession) {
	request := &xgress.Request{ServiceId: l.service}
	response := xgress.CreateSession(l.ctrl, session, request, l.bindHandler, l.options)
	if !response.Success{
		logrus.Errorf("error creating session (%s)", response.Message)
		_ = session.Close()
	}
}

func (l *listener) close() {
	if l.conn != nil {
		if err := l.conn.Close(); err != nil {
			logrus.Errorf("error closing packet connection (%w)", err)
		}
	}
}

func newListener(service string, ctrl xgress.CtrlChannel, options *xgress.Options) xgress.XgressListener {
	return &listener{
		service:   service,
		ctrl:      ctrl,
		options:   options,
		dataChan:  make(chan *packetData, 10),
		eventChan: make(chan packetEvent, 10),
		sessions:  make(map[string]*packetSession),
	}
}

type listener struct {
	service     string
	ctrl        xgress.CtrlChannel
	options     *xgress.Options
	address     string
	bindHandler xgress.BindHandler
	conn        net.PacketConn
	dataChan    chan *packetData
	eventChan   chan packetEvent
	sessions    map[string]*packetSession
}

type packetData struct {
	buffer []byte
	source net.Addr
}

type packetEvent interface {
	handle(listener *listener)
}

func (s *packetSession) ReadPayload() ([]byte, map[uint8][]byte, error) {
	select {
	case buffer, chanOpen := <-s.readC:
		if !chanOpen {
			return buffer, nil, io.EOF
		}
		return buffer, nil, nil
	}
}

func (s *packetSession) WritePayload(p []byte, _ map[uint8][]byte) (n int, err error) {
	return s.Write(p)
}

func (s *packetSession) Write(p []byte) (n int, err error) {
	s.listener.eventChan <- (*sessionUpdateEvent)(s)
	return s.listener.conn.WriteTo(p, s.addr)
}

func (s *packetSession) LogContext() string {
	return s.addr.String()
}

func (s *packetSession) Close() error {
	s.listener.eventChan <- (*sessionCloseEvent)(s)
	return nil
}

func (s *packetSession) MarkActivity() {
	s.timeoutNanos = time.Now().UnixNano() + s.timeoutIntervalNanos
}

func (s *packetSession) getSessionId() string {
	return s.addr.String()
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

type sessionState uint8

const (
	sessionStateNew sessionState = iota
	sessionStateEstablished
)

type sessionUpdateEvent packetSession

func (s *sessionUpdateEvent) handle(l *listener) {
	(*packetSession)(s).MarkActivity()
}

type sessionCloseEvent packetSession

func (e *sessionCloseEvent) handle(l *listener) {
	session := (*packetSession)(e)
	if !session.closed {
		close(session.readC)
		delete(l.sessions, session.getSessionId())
		session.closed = true
	}
}
