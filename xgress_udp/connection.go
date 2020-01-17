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
	"github.com/netfoundry/ziti-foundation/transport/udp"
	"net"
)

type xgressPacketConn struct {
	net.Conn
}

func (xpc *xgressPacketConn) LogContext() string {
	return xpc.RemoteAddr().String()
}

func (xpc *xgressPacketConn) ReadPayload() ([]byte, map[uint8][]byte, error) {
	buffer := make([]byte, udp.MaxPacketSize)
	n, err := xpc.Read(buffer)
	if err != nil {
		return nil, nil, err
	}
	return buffer[:n], nil, nil
}

func (xpc *xgressPacketConn) WritePayload(p []byte, headers map[uint8][]byte) (n int, err error) {
	return xpc.Write(p)
}
