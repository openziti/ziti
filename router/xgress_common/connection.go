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

package xgress_common

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/secretstream"
	"github.com/netfoundry/secretstream/kx"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/sdk-golang/ziti/edge/impl"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
)

const (
	closedFlag      = 0
	sentFinFlag     = 1
	recvFinFlag     = 2
	outOfBandTxFlag = 3
	halfCloseFlag   = 4
	writeClosedFlag = 5
	xgressTypeFlag  = 6
)

type outOfBand struct {
	data    []byte
	headers map[uint8][]byte
}

type XgressConn struct {
	net.Conn

	rxKey       []byte
	outOfBandTx chan *outOfBand
	receiver    secretstream.Decryptor
	sender      secretstream.Encryptor

	writeDone chan struct{}
	flags     concurrenz.AtomicBitSet
}

func NewXgressConn(conn net.Conn, halfClose bool, isTransport bool) *XgressConn {
	result := &XgressConn{
		Conn:        conn,
		outOfBandTx: make(chan *outOfBand, 1),
		writeDone:   make(chan struct{}),
	}
	result.flags.Set(halfCloseFlag, halfClose)
	result.flags.Set(xgressTypeFlag, isTransport)
	return result
}

func (self *XgressConn) CloseWrite() error {
	if self.flags.IsSet(halfCloseFlag) {
		if self.flags.CompareAndSet(sentFinFlag, false, true) {
			self.outOfBandTx <- &outOfBand{
				headers: GetFinHeaders(),
			}
			self.flags.Set(outOfBandTxFlag, true)
		}
		return nil
	}

	return self.Close()
}

func (self *XgressConn) SetupClientCrypto(keyPair *kx.KeyPair, peerKey []byte) error {
	return self.setupCrypto(keyPair, peerKey, true)
}

func (self *XgressConn) SetupServerCrypto(peerKey []byte) ([]byte, error) {
	keyPair, err := kx.NewKeyPair()
	if err != nil {
		return nil, errors.Wrap(err, "unable to setup encryption for connection")
	}
	if err = self.setupCrypto(keyPair, peerKey, false); err != nil {
		return nil, err
	}
	return keyPair.Public(), nil
}

func (self *XgressConn) setupCrypto(keyPair *kx.KeyPair, peerKey []byte, client bool) error {
	var rx, tx []byte
	var err error

	if client {
		if rx, tx, err = keyPair.ClientSessionKeys(peerKey); err != nil {
			return errors.Wrap(err, "failed key exchange")
		}
	} else {
		if rx, tx, err = keyPair.ServerSessionKeys(peerKey); err != nil {
			return errors.Wrap(err, "failed key exchange")
		}
	}

	var txHeader []byte
	if self.sender, txHeader, err = secretstream.NewEncryptor(tx); err != nil {
		return errors.Wrap(err, "failed to establish crypto stream")
	}

	self.outOfBandTx <- &outOfBand{data: txHeader}
	self.flags.Set(outOfBandTxFlag, true)

	self.rxKey = rx

	pfxlog.Logger().Debug("crypto established")
	return nil
}

func (self *XgressConn) LogContext() string {
	return self.Conn.RemoteAddr().String()
}

func (self *XgressConn) IsClosed() bool {
	return self.flags.IsSet(closedFlag)
}

func (self *XgressConn) IsWriteClosed() bool {
	return self.flags.IsSet(sentFinFlag)
}

func (self *XgressConn) ReadPayload() ([]byte, map[uint8][]byte, error) {
	// First thing we need to do is send the encryption header, if one exists

	if self.flags.Load() != 0 {
		if self.flags.IsSet(outOfBandTxFlag) {
			var oob *outOfBand
			select {
			case oob = <-self.outOfBandTx:
				return oob.data, oob.headers, nil
			default:
				self.flags.Set(outOfBandTxFlag, false)
			}
		}

		if self.IsWriteClosed() {
			<-self.writeDone
			return nil, nil, io.EOF
		}

		if self.IsClosed() {
			return nil, nil, io.EOF
		}
	}

	buffer := make([]byte, 10240)
	n, err := self.Conn.Read(buffer)
	buffer = buffer[:n]

	if self.sender != nil && n > 0 {
		var cryptoErr error
		buffer, cryptoErr = self.sender.Push(buffer, secretstream.TagMessage)
		if cryptoErr != nil {
			if err == nil {
				err = cryptoErr
			} else {
				err = impl.MultipleErrors{err, cryptoErr}
			}
		}
	}

	if err != nil && n == 0 && errors.Is(err, io.EOF) && self.flags.IsSet(halfCloseFlag) {
		if self.flags.CompareAndSet(sentFinFlag, false, true) {
			return nil, GetFinHeaders(), nil
		}
	}

	return buffer, nil, err
}

func (self *XgressConn) WritePayload(p []byte, headers map[uint8][]byte) (int, error) {
	if self.flags.IsSet(recvFinFlag) {
		return 0, errors.New("calling WritePayload() after CloseWrite()")
	}

	if flags, found := headers[PayloadFlagsHeader]; found {
		if flags[0]&edge.FIN != 0 {
			defer func() {
				conn, ok := self.Conn.(edge.CloseWriter)
				// if connection does not support half-close just let xgress tear it down
				if ok {
					_ = conn.CloseWrite()
				} else {
					_ = self.Conn.Close()
				}
				self.notifyWriteDone()
			}()
		}
	}

	var err error

	// first data message should contain crypto header
	if self.rxKey != nil {
		if len(p) != secretstream.StreamHeaderBytes {
			return 0, errors.Errorf("failed to receive crypto header bytes: read[%d]", len(p))
		}
		self.receiver, err = secretstream.NewDecryptor(self.rxKey, p)
		self.rxKey = nil
		return len(p), err
	}

	if len(p) > 0 {
		buf := p
		if self.receiver != nil {
			buf, _, err = self.receiver.Pull(p)
			if err != nil {
				log.WithError(err).Errorf("crypto failed on msg of size=%v, headers=%+v", len(p), headers)
				return 0, err
			}
		}

		_, err = self.Write(buf)
		if err != nil {
			_ = self.Close()
		}
		return len(p), err
	}
	return 0, nil
}

func (self *XgressConn) notifyWriteDone() {
	if self.flags.CompareAndSet(writeClosedFlag, false, true) {
		self.flags.Set(recvFinFlag, true)
		close(self.writeDone)
	}
}

func (self *XgressConn) Close() error {
	if self.flags.CompareAndSet(closedFlag, false, true) {
		self.notifyWriteDone()
		return self.Conn.Close()
	}
	return nil
}

func (self *XgressConn) HandleControlMsg(controlType xgress.ControlType, headers channel.Headers, responder xgress.ControlReceiver) error {
	if controlType == xgress.ControlTypeTraceRoute {
		hopType := "xgress/edge_transport"
		if !self.flags.IsSet(xgressTypeFlag) {
			hopType = "xgress/tunnel"
		}
		// TODO: find a way to get terminator id for hopId
		xgress.RespondToTraceRequest(headers, hopType, "", responder)
		return nil
	}
	return errors.Errorf("unhandled control type: %v", controlType)
}
