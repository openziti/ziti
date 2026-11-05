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

package xgress_transport_udp

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/router/xgress_router"
	"github.com/openziti/ziti/v2/router/xgress_udp"
	"github.com/stretchr/testify/require"
)

// Test_SessionResponse_ClosedBeforeResponse locks in that when a new circuit closes before handleConnect
// queues its success response, the loop handles the close first, so the client is told the connect
// failed and the session is not marked established.
func Test_SessionResponse_ClosedBeforeResponse(t *testing.T) {
	req := require.New(t)

	// arrange: the circuit's xgress closes its peer, then handleConnect queues the response
	l := newEventLoopListener()
	session := l.newSession()
	req.NoError(session.Close())
	l.QueueEvent(&sessionResponse{session: session, response: &xgress_router.Response{Success: true, CircuitId: "c1"}})

	// act
	for len(l.events) > 0 {
		(<-l.events).Handle(l)
	}

	// assert
	req.NotEqual(xgress_udp.SessionStateEstablished, session.State(), "a closed session must not be marked established")
	_, found := l.GetSession(session.SessionId())
	req.False(found, "the close must remove the session")
	sent, err := l.sentResponse()
	req.NoError(err)
	req.False(sent.Success, "a closed session must not be reported as connected")
}

// Test_SessionResponse_RemovedSession locks in that a connect response whose session expired with no
// replacement still tells the client it timed out.
func Test_SessionResponse_RemovedSession(t *testing.T) {
	req := require.New(t)

	// arrange
	l := newEventLoopListener()
	expired := l.newSession()
	expired.CloseFromEventLoop()
	response := &sessionResponse{session: expired, response: &xgress_router.Response{Success: true, CircuitId: "c1"}}

	// act
	response.Handle(l)

	// assert
	sent, err := l.sentResponse()
	req.NoError(err)
	req.False(sent.Success, "a response for an expired session must not report a connection")
	req.Equal("timeout", sent.Message, "the client must be told its connect timed out")
}

// Test_SessionResponse_ReplacedSession locks in that a connect response whose session expired and was
// replaced by a new session from the same address leaves the replacement alone and sends the client
// nothing, since the client would take any reply as the answer to its new attempt.
func Test_SessionResponse_ReplacedSession(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(fmt.Sprintf("a stale response with success=%v leaves the replacement alone", success), func(t *testing.T) {
			req := require.New(t)

			// arrange
			l := newEventLoopListener()
			expired := l.newSession()
			expired.CloseFromEventLoop()
			replacement := l.newSession()
			response := &sessionResponse{session: expired, response: &xgress_router.Response{Success: success, CircuitId: "c1"}}

			// act
			response.Handle(l)

			// assert
			current, found := l.GetSession(replacement.SessionId())
			req.True(found, "a stale response must not remove the replacement session")
			req.Same(replacement, current, "a stale response must not remove the replacement session")
			req.Equal(xgress_udp.SessionStateNew, replacement.State(), "a stale response must not change the replacement's state")
			req.Empty(l.written, "a stale response must not reach the client")
		})
	}
}

// eventLoopListener stands in for a listener's event loop: the test handles queued events itself.
type eventLoopListener struct {
	sessions map[string]xgress_udp.Session
	events   chan xgress_udp.EventHandler
	written  chan []byte
}

func newEventLoopListener() *eventLoopListener {
	return &eventLoopListener{
		sessions: map[string]xgress_udp.Session{},
		events:   make(chan xgress_udp.EventHandler, 10),
		written:  make(chan []byte, 10),
	}
}

func (l *eventLoopListener) Close() error {
	return nil
}

func (l *eventLoopListener) WriteTo(data []byte, _ net.Addr) (int, error) {
	l.written <- data
	return len(data), nil
}

func (l *eventLoopListener) GetSession(sessionId string) (xgress_udp.Session, bool) {
	session, found := l.sessions[sessionId]
	return session, found
}

func (l *eventLoopListener) DeleteSession(sessionId string) {
	delete(l.sessions, sessionId)
}

func (l *eventLoopListener) QueueEvent(event xgress_udp.EventHandler) {
	l.events <- event
}

func (l *eventLoopListener) LogContext() string {
	return "test"
}

// newSession returns a new session for the test client's address, registered with l as that address's
// current session.
func (l *eventLoopListener) newSession() xgress_udp.Session {
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 10000}
	session := xgress_udp.NewPacketSesssion(l, addr, time.Minute.Nanoseconds())
	l.sessions[session.SessionId()] = session
	return session
}

// sentResponse returns the next connect response written to the client, or an error if none was
// written.
func (l *eventLoopListener) sentResponse() (*xgress_router.Response, error) {
	select {
	case written := <-l.written:
		return xgress_router.ResponseFromJSON(bytes.TrimSuffix(written, []byte{'\n'}))
	default:
		return nil, errors.New("no response was sent to the client")
	}
}
