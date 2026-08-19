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

package xgress_common

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/xgress"
	"github.com/stretchr/testify/require"
)

// eofConn is a connection whose reads have run out. Its deadline setters succeed, matching the
// datagram connections that implement them as no-ops, so the liveness probe in ReadPayload
// cannot tell that the peer is gone.
type eofConn struct {
	net.Conn
}

func (self *eofConn) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (self *eofConn) Write(b []byte) (int, error) {
	return len(b), nil
}

func (self *eofConn) Close() error {
	return nil
}

func (self *eofConn) SetReadDeadline(time.Time) error  { return nil }
func (self *eofConn) SetWriteDeadline(time.Time) error { return nil }

// TestReadPayloadEofWithoutHalfClose covers a connection with no half-close semantics reaching
// end of input. There is nothing further the local side can signal, so this has to be reported
// as a full close. Reporting a bare io.EOF instead reads as a half-close to Xgress.rx(), which
// leaves the xgress waiting on an end-of-circuit that will never arrive.
func TestReadPayloadEofWithoutHalfClose(t *testing.T) {
	req := require.New(t)

	conn := NewXgressConn(&eofConn{}, false, ConnTypeTunnel)

	data, headers, err := conn.ReadPayload()
	req.ErrorIs(err, xgress.ErrPeerClosed)
	req.Nil(data)
	req.Nil(headers)

	// still reported on a subsequent read, rather than reverting to a half-close
	_, _, err = conn.ReadPayload()
	req.ErrorIs(err, xgress.ErrPeerClosed)
}

// TestReadPayloadEofWithHalfClose covers the half-close path, which must keep reporting the FIN
// headers followed by io.EOF so the write half can close while the read half stays open.
func TestReadPayloadEofWithHalfClose(t *testing.T) {
	req := require.New(t)

	conn := NewXgressConn(&eofConn{}, true, ConnTypeTunnel)

	data, headers, err := conn.ReadPayload()
	req.NoError(err)
	req.Nil(data)
	req.Equal(GetFinHeaders(), headers, "first read at end of input should carry the fin headers")

	data, headers, err = conn.ReadPayload()
	req.ErrorIs(err, io.EOF)
	req.Empty(data)
	req.Nil(headers)
	req.False(conn.IsClosed(), "half-close should leave the conn open for reading")
}
