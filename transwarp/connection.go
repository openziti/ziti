/*
	(c) Copyright NetFoundry, Inc.

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

package transwarp

import (
	"crypto/x509"
	"github.com/netfoundry/ziti-foundation/transport"
	"io"
	"net"
	"time"
)

func (self *connection) Detail() *transport.ConnectionDetail {
	return self.detail
}

func (_ *connection) PeerCertificates() []*x509.Certificate {
	return nil
}

func (self *connection) Reader() io.Reader {
	return self.socket
}

func (self *connection) Writer() io.Writer {
	return self.socket
}

func (self *connection) Conn() net.Conn {
	return self.socket
}

func (self *connection) SetReadTimeout(t time.Duration) error {
	return self.socket.SetReadDeadline(time.Now().Add(t))
}

func (self *connection) SetWriteTimeout(t time.Duration) error {
	return self.socket.SetWriteDeadline(time.Now().Add(t))
}

func (self *connection) Close() error {
	return self.socket.Close()
}

func newConnection(detail *transport.ConnectionDetail, socket net.Conn) *connection {
	return &connection{
		detail: detail,
		socket: socket,
		buffer: make([]byte, 4096),
	}
}

type connection struct {
	detail *transport.ConnectionDetail
	socket net.Conn
	buffer []byte
	copy   []byte
}
