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
	"errors"
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
		var ok bool

		select {
		case buf, ok = <-conn.readC:
		case <-conn.closeNotify:
		}

		if !ok {
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
	return 0, errors.New("read not implemented, assuming we always want WriteTo used instead")
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
