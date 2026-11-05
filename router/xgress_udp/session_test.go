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

package xgress_udp

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test_PacketSession_CallsFromListenerLoop locks in that the calls the listener's event loop makes on its
// own sessions, Write, QueueRead and CloseFromEventLoop, never wait on that loop, and that a read waiting
// on a stopped reader is released when the session closes.
func Test_PacketSession_CallsFromListenerLoop(t *testing.T) {
	t.Run("a write returns while the event loop is busy", func(t *testing.T) {
		// arrange
		session := newTestSession(newStalledListener())

		// act
		written := make(chan struct{})
		go write(session, []byte("response"), 1, written)

		// assert
		requireClosed(t, written, "a write must not wait on the listener's event loop")
	})

	t.Run("a read past a full buffer waits while the session is open", func(t *testing.T) {
		req := require.New(t)

		// arrange: the session's reader has stopped with the read buffer full
		l := newStalledListener()
		session := newTestSession(l)
		for range cap(session.readC) {
			session.QueueRead([]byte("packet"))
		}

		// act
		queued := make(chan struct{})
		go queueRead(session, []byte("packet"), queued)

		// assert
		select {
		case <-queued:
			req.FailNow("a read past a full buffer must wait for the reader while the session is open")
		case <-time.After(100 * time.Millisecond):
		}

		t.Run("closing the session releases the waiting read", func(t *testing.T) {
			req := require.New(t)

			// act: the reader's xgress closes the session from its own goroutine, which waits on the busy loop
			closed := make(chan struct{})
			go closeSession(session, closed)

			// assert
			requireClosed(t, queued, "closing the session must release the waiting read")
			select {
			case event := <-l.events:
				event.Handle(l)
			case <-time.After(5 * time.Second):
				req.FailNow("closing the session must queue its removal on the event loop")
			}
			req.Equal(session.SessionId(), <-l.deleted, "the queued removal must delete the session")
			requireClosed(t, closed, "the close must return once the loop takes its removal")
		})
	})

	t.Run("closing from the event loop removes the session without waiting on the loop", func(t *testing.T) {
		req := require.New(t)

		// arrange
		l := newStalledListener()
		session := newTestSession(l)

		// act
		closed := make(chan struct{})
		go closeFromEventLoop(session, closed)

		// assert
		requireClosed(t, closed, "closing from the event loop must not wait on the loop")
		req.Len(l.deleted, 1, "the session must be removed before CloseFromEventLoop returns")
		req.Equal(session.SessionId(), <-l.deleted)
		_, _, err := session.ReadPayload()
		req.ErrorIs(err, io.EOF, "a removed session's reader must see the end of the stream")

		t.Run("a later close returns without waiting on the loop", func(t *testing.T) {
			// act
			closed := make(chan struct{})
			go closeSession(session, closed)

			// assert
			requireClosed(t, closed, "closing a session the loop already removed must not queue its removal")
		})
	})
}

// Test_PacketSession_ClosingSessionDeliversReadsThatFit locks in that between Close and the event loop
// removing the session, reads that fit in the read buffer still reach the reader.
func Test_PacketSession_ClosingSessionDeliversReadsThatFit(t *testing.T) {
	req := require.New(t)

	// arrange: the session is closed and its removal is still waiting for the event loop
	session := newTestSession(&discardingListener{})
	req.NoError(session.Close())

	// act
	for range cap(session.readC) {
		session.QueueRead([]byte("packet"))
	}

	// assert
	req.Len(session.readC, cap(session.readC), "every read that fits must be delivered")
}

// Test_PacketSession_WriteMarksActivity locks in that a write marks the session active itself, on the
// writer's goroutine, while the event loop reads the timeout concurrently. It relies on -race to catch
// an unsynchronized timeout.
func Test_PacketSession_WriteMarksActivity(t *testing.T) {
	// arrange
	session := newTestSession(&discardingListener{})
	start := time.Now().UnixNano()

	// act
	written := make(chan struct{})
	go write(session, []byte("payload"), 100, written)
	for range 100 {
		_ = session.TimeoutNanos()
	}
	<-written

	// assert
	require.Greater(t, session.TimeoutNanos(), start, "a write must push the session's timeout past the time of the write")
}

// stalledListener is a Listener whose event loop is busy: QueueEvent blocks until the test receives
// from events.
type stalledListener struct {
	events  chan EventHandler
	deleted chan string
}

func newStalledListener() *stalledListener {
	return &stalledListener{events: make(chan EventHandler), deleted: make(chan string, 1)}
}

func (l *stalledListener) Close() error {
	return nil
}

func (l *stalledListener) WriteTo(data []byte, _ net.Addr) (int, error) {
	return len(data), nil
}

func (l *stalledListener) GetSession(string) (Session, bool) {
	return nil, false
}

func (l *stalledListener) DeleteSession(sessionId string) {
	l.deleted <- sessionId
}

func (l *stalledListener) QueueEvent(event EventHandler) {
	l.events <- event
}

func (l *stalledListener) LogContext() string {
	return "test"
}

// discardingListener is a Listener whose event loop drops every queued event, as if it never handled
// them.
type discardingListener struct {
	stalledListener
}

func (l *discardingListener) QueueEvent(EventHandler) {
}

// newTestSession returns a session on l for a fixed client address, with a one-minute timeout.
func newTestSession(l Listener) *PacketSession {
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 10000}
	return NewPacketSesssion(l, addr, time.Minute.Nanoseconds()).(*PacketSession)
}

// write writes data to session count times, then closes done.
func write(session *PacketSession, data []byte, count int, done chan<- struct{}) {
	defer close(done)
	for range count {
		_, _ = session.Write(data)
	}
}

// queueRead queues data for session's reader, then closes done.
func queueRead(session *PacketSession, data []byte, done chan<- struct{}) {
	defer close(done)
	session.QueueRead(data)
}

// closeSession closes session, then closes done.
func closeSession(session *PacketSession, done chan<- struct{}) {
	defer close(done)
	_ = session.Close()
}

// closeFromEventLoop closes session as the listener's event loop does, then closes done.
func closeFromEventLoop(session *PacketSession, done chan<- struct{}) {
	defer close(done)
	session.CloseFromEventLoop()
}

// requireClosed fails the test unless done is closed within five seconds.
func requireClosed(t *testing.T, done <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, msg)
	}
}
