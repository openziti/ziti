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
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/tunnel"
	"github.com/openziti/ziti/tunnel/entities"
	"github.com/stretchr/testify/require"
)

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

// createWriteQueueEvent creates a write queue on the manager's event loop, where the manager expects
// it, and hands the connection back.
type createWriteQueueEvent struct {
	service *entities.Service
	writer  UDPWriterTo
	result  chan *udpConn
}

func (self *createWriteQueueEvent) Handle(manager Manager) error {
	target := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8690}
	client := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 40000}
	writeQueue, err := manager.CreateWriteQueue(target, client, self.service, self.writer)
	if err != nil {
		return err
	}
	self.result <- writeQueue.(*udpConn)
	return nil
}

// Test_udpConn_Close verifies a virtual connection closes the write conn only when the manager doesn't
// share it. A proxy shares one listener across every client of a service, so a client's connection
// closing it stops the service's UDP intercept.
func Test_udpConn_Close(t *testing.T) {
	for _, shared := range []bool{true, false} {
		t.Run(fmt.Sprintf("sharedWriteConn=%v", shared), func(t *testing.T) {
			req := require.New(t)

			provider := &noopDialProvider{}
			manager := NewManager(provider, NewUnlimitedConnectionPolicy(), NewDefaultExpirationPolicy(), shared)

			name := "test-service"
			event := &createWriteQueueEvent{
				service: &entities.Service{FabricProvider: provider, ServiceDetail: rest_model.ServiceDetail{Name: &name}},
				writer:  &closeTrackingWriter{},
				result:  make(chan *udpConn, 1),
			}
			manager.QueueEvent(event)

			var conn *udpConn
			select {
			case conn = <-event.result:
			case <-time.After(5 * time.Second):
				req.Fail("timed out waiting for the write queue")
			}

			req.NoError(conn.Close())
			req.Equal(!shared, event.writer.(*closeTrackingWriter).closed.Load())
		})
	}
}
