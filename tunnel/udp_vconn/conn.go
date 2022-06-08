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

package udp_vconn

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/mempool"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"sync/atomic"
	"time"
)

type udpConn struct {
	readC       chan mempool.PooledBuffer
	closeNotify chan struct{}
	service     string
	srcAddr     net.Addr
	manager     *manager
	writeConn   UDPWriterTo
	lastUse     atomic.Value
	closed      concurrenz.AtomicBoolean
	leftOver    []byte
	leftOverBuf mempool.PooledBuffer
}

func (conn *udpConn) Service() string {
	return conn.service
}

func (conn *udpConn) Accept(buffer mempool.PooledBuffer) {
	logrus.WithField("udpConnId", conn.srcAddr.String()).Debugf("udp->ziti: queuing")
	select {
	case conn.readC <- buffer:
	case <-conn.closeNotify:
		buffer.Release()
		logrus.WithField("udpConnId", conn.srcAddr.String()).Debugf("udp->ziti: closed, cancelling accept")
	}
}

func (conn *udpConn) Network() string {
	return "ziti"
}

func (conn *udpConn) String() string {
	return conn.service
}

func (conn *udpConn) markUsed() {
	conn.lastUse.Store(time.Now())
}

func (conn *udpConn) GetLastUsed() time.Time {
	val := conn.lastUse.Load()
	return val.(time.Time)
}

func (conn *udpConn) WriteTo(w io.Writer) (n int64, err error) {
	var bytesWritten int64
	for {
		var buf mempool.PooledBuffer

		select {
		case buf = <-conn.readC:
		case <-conn.closeNotify:
			select {
			case buf = <-conn.readC:
			default:
			}
		}

		if buf == nil {
			return bytesWritten, io.EOF
		}

		payload := buf.GetPayload()
		pfxlog.Logger().WithField("udpConnId", conn.srcAddr.String()).Debugf("udp->ziti: %v bytes", len(payload))
		n, err := w.Write(payload)
		buf.Release()
		conn.markUsed()
		bytesWritten += int64(n)
		if err != nil {
			return bytesWritten, err
		}
	}
}

func (conn *udpConn) Read(b []byte) (n int, err error) {
	leftOverLen := len(conn.leftOver)
	if leftOverLen > 0 {
		copy(b, conn.leftOver)
		if leftOverLen > len(b) {
			conn.leftOver = conn.leftOver[len(b):]
			conn.markUsed()
			return len(b), nil
		}

		conn.leftOver = nil
		conn.leftOverBuf.Release()
		conn.leftOverBuf = nil

		conn.markUsed()
		return leftOverLen, nil
	}

	var bytesWritten int

	var buf mempool.PooledBuffer

	select {
	case buf = <-conn.readC:
	case <-conn.closeNotify:
		select {
		case buf = <-conn.readC:
		default:
		}
	}

	if buf == nil {
		conn.markUsed()
		return bytesWritten, io.EOF
	}

	data := buf.GetPayload()
	dataLen := len(data)
	copy(b, data)
	if dataLen <= len(b) {
		buf.Release()
		conn.markUsed()
		return dataLen, nil
	}

	conn.leftOver = data[len(b):]
	conn.leftOverBuf = buf
	conn.markUsed()

	return len(b), nil
}

func (conn *udpConn) Write(b []byte) (int, error) {
	pfxlog.Logger().WithField("udpConnId", conn.srcAddr.String()).Debugf("ziti->udp: %v bytes", len(b))
	// TODO: UDP chunking, MTU chunking?
	n, err := conn.writeConn.WriteTo(b, conn.srcAddr)
	conn.markUsed()
	return n, err
}

func (conn *udpConn) Close() error {
	if conn.closed.CompareAndSwap(false, true) {
		close(conn.closeNotify)
		if err := conn.writeConn.Close(); err != nil {
			logrus.WithField("service", conn.service).
				WithField("src_addr", conn.srcAddr).
				WithError(err).Error("error while closing udp connection")
		}
	}

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
