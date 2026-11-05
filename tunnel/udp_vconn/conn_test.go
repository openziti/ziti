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

package udp_vconn

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/tunnel"
	"github.com/openziti/ziti/v2/tunnel/entities"
	"github.com/stretchr/testify/require"
)

// Test_udpConn_Close locks in that a virtual connection closes the write conn only when it owns it. A
// proxy shares one listener across every client of a service, so a client's connection closing it would
// stop the service's UDP intercept.
func Test_udpConn_Close(t *testing.T) {
	for _, tc := range []struct {
		name            string
		sharedWriteConn bool
		wantClosed      bool
	}{
		{name: "a connection leaves a shared write conn open", sharedWriteConn: true, wantClosed: false},
		{name: "a connection closes a write conn it owns", sharedWriteConn: false, wantClosed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := require.New(t)

			// arrange
			provider := &noopDialProvider{}
			manager := NewManager(provider, NewUnlimitedConnectionPolicy(), NewDefaultExpirationPolicy(), tc.sharedWriteConn)
			name := "test-service"
			writer := &closeTrackingWriter{}
			event := &createWriteQueueEvent{
				service: &entities.Service{FabricProvider: provider, ServiceDetail: rest_model.ServiceDetail{Name: &name}},
				writer:  writer,
				done:    make(chan error, 1),
			}
			manager.QueueEvent(event)
			select {
			case err := <-event.done:
				req.NoError(err)
			case <-time.After(5 * time.Second):
				req.FailNow("the manager's event loop did not create the write queue")
			}

			// act
			err := event.conn.Close()

			// assert
			req.NoError(err)
			req.Equal(tc.wantClosed, writer.closed.Load(), "the write conn must be closed only when the connection owns it")
		})
	}
}

// closeTrackingWriter stands in for the intercept's packet conn and records whether it was closed.
type closeTrackingWriter struct {
	closed atomic.Bool
}

func (self *closeTrackingWriter) Close() error {
	self.closed.Store(true)
	return nil
}

func (self *closeTrackingWriter) WriteTo(b []byte, _ net.Addr) (int, error) { return len(b), nil }

func (self *closeTrackingWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8688}
}

// noopDialProvider accepts every dial and leaves the virtual connection open.
type noopDialProvider struct {
	tunnel.FabricProvider
}

func (self *noopDialProvider) TunnelService(tunnel.Service, string, net.Conn, bool, []byte) error {
	return nil
}

// createWriteQueueEvent creates a write queue on the manager's event loop, where the manager expects it,
// then sends the outcome on done. Once done yields a nil error, conn holds the new connection.
type createWriteQueueEvent struct {
	service *entities.Service
	writer  UDPWriterTo
	conn    *udpConn
	done    chan error
}

func (self *createWriteQueueEvent) Handle(manager Manager) error {
	target := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8690}
	client := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 40000}
	writeQueue, err := manager.CreateWriteQueue(target, client, self.service, self.writer)
	if err == nil {
		self.conn = writeQueue.(*udpConn)
	}
	self.done <- err
	return err
}
