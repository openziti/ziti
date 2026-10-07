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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

func newTestSession(l Listener) *PacketSession {
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 10000}
	return NewPacketSesssion(l, addr, time.Minute.Nanoseconds()).(*PacketSession)
}

func requireReturns(t *testing.T, f func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "call blocked on the listener's event loop or the session's reader")
	}
}

// Test_PacketSession_CallsFromListenerLoop: the listener's event loop calls Write, QueueRead and
// CloseFromEventLoop on its own sessions, so none of them may wait for that loop or for the session's
// reader.
func Test_PacketSession_CallsFromListenerLoop(t *testing.T) {
	t.Run("write", func(t *testing.T) {
		session := newTestSession(newStalledListener())
		requireReturns(t, func() {
			_, _ = session.Write([]byte("response"))
		})
	})

	t.Run("queue read past a stopped reader", func(t *testing.T) {
		req := require.New(t)
		l := newStalledListener()
		session := newTestSession(l)
		requireReturns(t, func() {
			for i := 0; i < cap(session.readC); i++ {
				session.QueueRead([]byte("packet"))
			}
		})

		queued := make(chan struct{})
		go func() {
			defer close(queued)
			session.QueueRead([]byte("packet"))
		}()

		select {
		case <-queued:
			req.FailNow("a read past a full buffer must wait for the reader while the session is open")
		case <-time.After(100 * time.Millisecond):
		}

		// The reader's xgress closes the session from its own goroutine, which waits on the busy loop.
		go func() {
			_ = session.Close()
		}()

		select {
		case <-queued:
		case <-time.After(5 * time.Second):
			req.FailNow("closing the session did not release the waiting read")
		}

		(<-l.events).Handle(l)
		req.Equal(session.SessionId(), <-l.deleted)
	})

	t.Run("close from event loop", func(t *testing.T) {
		req := require.New(t)
		l := newStalledListener()
		session := newTestSession(l)
		requireReturns(t, session.CloseFromEventLoop)

		select {
		case id := <-l.deleted:
			req.Equal(session.SessionId(), id)
		default:
			req.FailNow("the session must be removed before CloseFromEventLoop returns")
		}
		_, _, err := session.ReadPayload()
		req.ErrorIs(err, io.EOF)

		requireReturns(t, func() {
			_ = session.Close()
		})
	})
}

// Test_PacketSession_ClosingSessionDeliversReadsThatFit: between Close and the loop removing the
// session, reads that fit in the buffer still reach the reader, as they did before Close released
// waiting reads.
func Test_PacketSession_ClosingSessionDeliversReadsThatFit(t *testing.T) {
	req := require.New(t)
	l := newStalledListener()
	session := newTestSession(l)

	go func() {
		_ = session.Close()
	}()
	<-session.closeNotify

	requireReturns(t, func() {
		for i := 0; i < cap(session.readC); i++ {
			session.QueueRead([]byte("packet"))
		}
	})
	req.Equal(cap(session.readC), len(session.readC), "every read that fits must be delivered")

	(<-l.events).Handle(l)
}

// discardingListener drops every queued event, as if its loop never handled them.
type discardingListener struct {
	stalledListener
}

func (l *discardingListener) QueueEvent(EventHandler) {
}

// Test_PacketSession_WriteMarksActivity: a write marks the session active itself, on the writer's
// goroutine, while the listener's loop reads the timeout concurrently.
func Test_PacketSession_WriteMarksActivity(t *testing.T) {
	session := newTestSession(&discardingListener{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, _ = session.Write([]byte("payload"))
		}
	}()
	for i := 0; i < 100; i++ {
		_ = session.TimeoutNanos()
	}
	wg.Wait()

	require.NotZero(t, session.TimeoutNanos(), "a write must mark the session active")
}
