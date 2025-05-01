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

package xgress_proxy_udp

import (
	"fmt"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/router/xgress_router"
	"github.com/openziti/ziti/router/xgress_udp"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net"
	"time"
)

func (l *listener) Listen(address string, bindHandler xgress.BindHandler) error {
	if address == "" {
		return errors.New("address must be specified for proxy_udp listeners")
	}
	l.address = address
	l.bindHandler = bindHandler

	packetAddress, err := xgress_udp.Parse(address)
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

func (l *listener) relay() {
	defer func() {
		if err := l.Close(); err != nil {
			logrus.Errorf("error closing packet connection (%v)", err)
		}
	}()

	for {
		buf := make([]byte, info.MaxUdpPacketSize)
		read, addr, err := l.conn.ReadFrom(buf)
		if err != nil {
			logrus.Errorf("error reading packet (%v)", err)
		}

		if read > 0 {
			l.dataChan <- xgress_udp.NewPacketData(buf[:read], addr)
		}
	}
}

func (l *listener) rx() {
	scanTicker := time.NewTicker(time.Second * 10)
	defer scanTicker.Stop()

	for {
		select {
		case data := <-l.dataChan:
			sessionId := data.Source.String()
			session, found := l.sessions[sessionId]
			if !found {
				logrus.Infof("session not found for [%s]", sessionId)
				session = xgress_udp.NewPacketSesssion(l, data.Source, time.Minute.Nanoseconds())
				session.MarkActivity()
				l.sessions[sessionId] = session
				l.handleConnect(session)

				if session.State() == xgress_udp.SessionStateEstablished {
					logrus.Infof("created session [%s] => [%s]", sessionId, session.SessionId())
				} else {
					logrus.Infof("session creation failed [%s]", sessionId)
				}
			}

			if session.State() == xgress_udp.SessionStateEstablished {
				session.MarkActivity()
				session.QueueRead(data.Buffer)
			} else {
				logrus.Warnf("dropping")
			}

		case event := <-l.eventChan:
			event.Handle(l)

		case tick := <-scanTicker.C:
			now := tick.UnixNano()
			for _, session := range l.sessions {
				if session.TimeoutNanos() < now {
					_ = session.Close()
				}
			}
		}
	}
}

func (l *listener) handleConnect(session xgress_udp.Session) {
	request := &xgress_router.Request{ServiceId: l.service}
	response := xgress_router.CreateCircuit(l.ctrl, session, request, l.bindHandler, l.options)
	if response.Success {
		session.SetState(xgress_udp.SessionStateEstablished)
	} else {
		logrus.Errorf("error creating session (%s)", response.Message)
		_ = session.Close()
	}
}

func (l *listener) Close() error {
	if l.conn != nil {
		return l.conn.Close()
	}
	return nil
}

func newListener(service string, ctrl env.NetworkControllers, options *xgress.Options) xgress_router.Listener {
	return &listener{
		service:   service,
		ctrl:      ctrl,
		options:   options,
		dataChan:  make(chan *xgress_udp.PacketData, 10),
		eventChan: make(chan xgress_udp.EventHandler, 10),
		sessions:  make(map[string]xgress_udp.Session),
	}
}

type listener struct {
	service     string
	ctrl        env.NetworkControllers
	options     *xgress.Options
	address     string
	bindHandler xgress.BindHandler
	conn        net.PacketConn
	dataChan    chan *xgress_udp.PacketData
	eventChan   chan xgress_udp.EventHandler
	sessions    map[string]xgress_udp.Session
}
