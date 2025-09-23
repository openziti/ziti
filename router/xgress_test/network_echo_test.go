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

package xgress_test

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/router/xgress_common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// echoServer implements a simple echo server that reads data and writes it back
type echoServer struct {
	listener net.Listener
	connC    chan net.Conn
}

func newEchoServer(port int) (*echoServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, err
	}

	server := &echoServer{
		listener: listener,
		connC:    make(chan net.Conn, 10),
	}

	go server.acceptLoop()

	return server, nil
}

func (s *echoServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			pfxlog.Logger().WithError(err).Debug("echo server accept error")
			return
		}

		s.connC <- conn
	}
}

func (s *echoServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	pfxlog.Logger().Debug("echo server handling new connection")

	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		if err == io.EOF {
			pfxlog.Logger().Debug("echo server received EOF, closing connection")
		} else {
			pfxlog.Logger().WithError(err).Error("echo server read error")
		}
		return
	}

	data := buffer[:n]
	pfxlog.Logger().Debugf("echo server received %d bytes: %s", n, string(data))

	// Echo the data back
	_, writeErr := conn.Write(data)
	if writeErr != nil {
		pfxlog.Logger().WithError(writeErr).Error("echo server write error")
		return
	}

	pfxlog.Logger().Debugf("echo server echoed %d bytes back", n)

	// Close the connection after echoing (server-side close)
	if closeWriter, ok := conn.(interface{ CloseWrite() error }); ok {
		time.Sleep(time.Second)
		pfxlog.Logger().Info("echo server performed CLOSE WRITE")
		_ = closeWriter.CloseWrite()
	} else {
		_ = conn.Close()
	}
}

func (s *echoServer) Close() error {
	return s.listener.Close()
}

func (s *echoServer) Address() string {
	return s.listener.Addr().String()
}

// TestXgressConnHalfClose tests xgress instances with real network connections
func TestXgressConnHalfClose(t *testing.T) {
	logOptions := pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/")
	pfxlog.GlobalInit(logrus.DebugLevel, logOptions)
	logrus.SetFormatter(pfxlog.NewFormatter(pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/")))

	// Start echo server
	server, err := newEchoServer(0) // Use any available port
	require.NoError(t, err)
	defer server.Close()

	serverAddr := server.Address()
	pfxlog.Logger().Infof("Echo server started on %s", serverAddr)

	// Create mock data plane adapter
	adapter := NewMockDataPlaneAdapter()
	defer adapter.Close()

	// Create client connection
	clientClientConn, err := net.Dial("tcp", serverAddr)
	require.NoError(t, err)

	// Create server connection (for the xgress pair)
	var clientServerConn net.Conn
	select {
	case clientServerConn = <-server.connC:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for server connection")
	}

	// Create client connection
	serverClientConn, err := net.Dial("tcp", serverAddr)
	require.NoError(t, err)

	// Create server connection (for the xgress pair)
	var serverServerConn net.Conn
	select {
	case serverServerConn = <-server.connC:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for server connection")
	}

	go server.handleConnection(serverClientConn)

	// Wrap connections with XgressConn
	clientXgressConn := xgress_common.NewXgressConn(clientServerConn, true, false)
	serverXgressConn := xgress_common.NewXgressConn(serverServerConn, true, false)

	// Set up addresses and circuit
	clientXgAddr := xgress.Address("client-addr")
	serverXgAddr := xgress.Address("server-addr")
	circuitId := "echo-circuit"

	// Create xgress options
	options := xgress.DefaultOptions()

	// Create xgress instances using the XgressConn connections
	clientXgress := xgress.NewXgress(circuitId, "test", clientXgAddr, clientXgressConn, xgress.Initiator, options, nil)
	serverXgress := xgress.NewXgress(circuitId, "test", serverXgAddr, serverXgressConn, xgress.Terminator, options, nil)

	// Set up data plane adapter
	clientXgress.SetDataPlaneAdapter(adapter)
	serverXgress.SetDataPlaneAdapter(adapter)

	// Register xgress instances
	adapter.RegisterXgress(clientXgress)
	adapter.RegisterXgress(serverXgress)

	// Connect circuit bidirectionally
	adapter.ConnectCircuit(circuitId, clientXgAddr, serverXgAddr)
	adapter.ConnectCircuit(circuitId, serverXgAddr, clientXgAddr)

	// Set up close handlers
	clientClosed := make(chan struct{})
	serverClosed := make(chan struct{})

	clientXgress.AddCloseHandler(xgress.CloseHandlerF(func(x *xgress.Xgress) {
		pfxlog.Logger().Info("client xgress closed")
		adapter.CloseCircuit(x.CircuitId())
		close(clientClosed)
	}))

	serverXgress.AddCloseHandler(xgress.CloseHandlerF(func(x *xgress.Xgress) {
		pfxlog.Logger().Info("server xgress closed")
		adapter.CloseCircuit(x.CircuitId())
		close(serverClosed)
	}))

	// Start both xgress instances
	clientXgress.Start()
	serverXgress.Start()

	// Send test data upstream (client -> server -> echo server)
	testMessage := "Hello, Echo Server!"
	pfxlog.Logger().Infof("sending upstream data: %s", testMessage)

	// We need to send data through the client connection which will be read by clientXgress
	// and forwarded to serverXgress, which will write it to the echo server
	_, err = clientClientConn.Write([]byte(testMessage))
	require.NoError(t, err)

	// The echo server should have echoed the data back to serverXgress,
	// which should forward it to clientXgress, which should write it to clientConn

	// Read the echoed data from client connection
	buffer := make([]byte, 1024)
	err = clientClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	require.NoError(t, err)
	n, err := clientClientConn.Read(buffer)
	if err != nil {
		t.Fatalf("error reading echoed data: %v", err)
	} else {
		echoedData := string(buffer[:n])
		pfxlog.Logger().Infof("Received echoed data: %s", echoedData)
		require.Equal(t, testMessage, echoedData, "Echoed data should match sent data")
	}

	_, err = clientClientConn.Read(buffer)
	require.ErrorIs(t, err, io.EOF)

	// Close client connection when we get EOF after successful read
	pfxlog.Logger().Info("Closing client connection")
	clientClientConn.Close()

	// Wait for xgress instances to close with timeout
	// Wait for both xgress instances to close
	select {
	case <-clientClosed:
		pfxlog.Logger().Info("Client xgress closed successfully")
	case <-time.After(35 * time.Second):
		t.Fatal("Timeout waiting for client xgress to close")
	}

	select {
	case <-serverClosed:
		pfxlog.Logger().Info("Server xgress closed successfully")
	case <-time.After(35 * time.Second):
		t.Fatal("Timeout waiting for server xgress to close")
	}

	pfxlog.Logger().Info("Network echo test completed successfully")
}
