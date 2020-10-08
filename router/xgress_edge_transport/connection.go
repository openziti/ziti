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

package xgress_edge_transport

import (
	"fmt"
	"github.com/netfoundry/secretstream"
	"github.com/openziti/foundation/transport"
	"io"
	"sync"
)

type readResult struct {
	data    []byte
	headers map[uint8]byte
	encrypt bool
	err     error
}

type edgeTransportXgressConn struct {
	transport.Connection
	cryptoCtx   *cryptoContext
	readCh      chan *readResult
	startRead   sync.Once
	startCrypto sync.Once
}

func newEdgeTransportXgressConn(conn transport.Connection) *edgeTransportXgressConn {
	return &edgeTransportXgressConn{
		Connection: conn,
		cryptoCtx:  nil,
		readCh:     make(chan *readResult),
	}
}

func (c *edgeTransportXgressConn) LogContext() string {
	return c.Detail().String()
}

func (c *edgeTransportXgressConn) readFromServer() {
	for {
		buffer := make([]byte, 10240)
		n, err := c.Reader().Read(buffer)

		result := &readResult{
			data:    buffer[:n],
			headers: nil,
			err:     err,
			encrypt: true,
		}

		c.readCh <- result

		if err == io.EOF {
			return
		}
	}
}

func (c *edgeTransportXgressConn) read() *readResult {
	return <-c.readCh
}

func (c *edgeTransportXgressConn) ReadPayload() ([]byte, map[uint8][]byte, error) {
	readResult := c.read()

	if readResult.err != nil {
		return nil, nil, readResult.err
	}

	if readResult.encrypt && c.cryptoCtx != nil && c.cryptoCtx.toClientEncryptor != nil {
		data, err := c.cryptoCtx.toClientEncryptor.Push(readResult.data, secretstream.TagMessage)
		if err != nil {
			return nil, nil, err
		}
		return data, nil, nil
	}

	return readResult.data, nil, nil
}

func (c *edgeTransportXgressConn) writeClearToClient(p []byte) {
	c.readCh <- &readResult{
		data:    p,
		headers: nil,
		err:     nil,
		encrypt: false,
	}
}

func (c *edgeTransportXgressConn) startServerRead() {
	c.startRead.Do(func() { //do once
		go c.readFromServer()
	})
}

func (c *edgeTransportXgressConn) Write(p []byte) (n int, err error) {
	//"write" from client to server
	if c.cryptoCtx != nil {
		//if crypto enabled and we have not setup e2e
		if c.cryptoCtx.rxKey != nil {
			if len(p) != secretstream.StreamHeaderBytes {
				return 0, fmt.Errorf("error establishing crypto: expected key length %d got %d", len(p), secretstream.StreamHeaderBytes)
			}

			c.cryptoCtx.fromClientDecryptor, err = secretstream.NewDecryptor(c.cryptoCtx.rxKey, p)
			if err != nil {
				return 0, fmt.Errorf("error establishing crypto: %v", err)
			}

			c.cryptoCtx.rxKey = nil

			c.writeClearToClient(c.cryptoCtx.publicTxHeader)

			//start reading from the server only after we have written to the client
			c.startServerRead()

			return len(p), nil //don't actually forward this just say we did
		} else {
			data, _, err := c.cryptoCtx.fromClientDecryptor.Pull(p)

			if err != nil {
				return 0, fmt.Errorf("could not decrypt data from client: %v", err)
			}

			p = data
		}
	} else {
		//no crypto start server reading
		c.startServerRead()
	}

	return c.Writer().Write(p)
}

func (c *edgeTransportXgressConn) WritePayload(p []byte, headers map[uint8][]byte) (n int, err error) {
	return c.Write(p)
}
