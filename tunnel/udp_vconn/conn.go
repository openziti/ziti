/*
	Copyright 2019 Netfoundry, Inc.

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

package udp_vconn

import (
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-foundation/util/mempool"
	"io"
	"net"
	"time"
)

type udpConn struct {
	readC     chan mempool.PooledBuffer
	service   string
	srcAddr   net.Addr
	manager   *manager
	writeConn UDPWriterTo
	lastUse   time.Time
	closed    bool
}

func (conn *udpConn) Service() string {
	return conn.service
}

func (conn *udpConn) Accept(buffer mempool.PooledBuffer) {
	conn.readC <- buffer
}

func (conn *udpConn) Network() string {
	return "ziti"
}

func (conn *udpConn) String() string {
	return conn.service
}

func (conn *udpConn) WriteTo(w io.Writer) (n int64, err error) {
	var bytesWritten int64
	for {
		buf, ok := <-conn.readC

		if !ok {
			return bytesWritten, io.EOF
		}

		n, err := w.Write(buf.GetPayload())
		buf.Release()
		conn.lastUse = time.Now()
		bytesWritten += int64(n)
		if err != nil {
			return bytesWritten, err
		}
	}
}

func (conn *udpConn) Read(b []byte) (n int, err error) {
	return 0, errors.New("read not implemented, assuming we always want WriteTo used instead")
}

func (conn *udpConn) Write(b []byte) (int, error) {
	pfxlog.Logger().Infof("Received %v bytes from ziti to return to udp", len(b))
	// TODO: UDP chunking, MTU chunking?
	n, err := conn.writeConn.WriteTo(b, conn.srcAddr)
	conn.lastUse = time.Now()
	return n, err
}

func (conn *udpConn) Close() error {
	conn.manager.queueClose(conn)
	return nil
}

func (conn *udpConn) LocalAddr() net.Addr {
	return conn.srcAddr
}

func (conn *udpConn) RemoteAddr() net.Addr {
	return conn
}

func (conn *udpConn) SetDeadline(t time.Time) error {
	// ignore, since this is a shared connection
	return nil
}

func (conn *udpConn) SetReadDeadline(t time.Time) error {
	// ignore, since this is a shared connection
	return nil
}

func (conn *udpConn) SetWriteDeadline(t time.Time) error {
	// ignore, since this is a shared connection
	return nil
}
