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
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/util/info"
	"github.com/pkg/errors"
	"net"
)

func (p *packetConn) LogContext() string {
	return p.RemoteAddr().String()
}

func (p *packetConn) ReadPayload() ([]byte, map[uint8][]byte, error) {
	buffer := make([]byte, info.MaxUdpPacketSize)
	n, err := p.Read(buffer)
	if err != nil {
		return nil, nil, err
	}
	return buffer[:n], nil, nil
}

func (p *packetConn) WritePayload(data []byte, headers map[uint8][]byte) (n int, err error) {
	return p.Write(data)
}

func (self *packetConn) HandleControlMsg(controlType xgress.ControlType, headers channel2.Headers, responder xgress.ControlReceiver) error {
	if controlType == xgress.ControlTypeTraceRoute {
		xgress.RespondToTraceRequest(headers, "xgress/transport_udp", "", responder)
		return nil
	}
	return errors.Errorf("unhandled control type: %v", controlType)
}

func newPacketConn(conn net.Conn) xgress.Connection {
	return &packetConn{conn}
}

type packetConn struct {
	net.Conn
}
