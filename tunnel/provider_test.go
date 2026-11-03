package tunnel

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/stretchr/testify/require"
)

// Test_accept locks in that the hosting accept loop hands each accepted conn off before dialing its
// upstream or completing the accept, so one slow conn can't hold up the conns accepted after it.
// Conns are accepted in order, so the later conn can only be handled first if the stalled one is
// running on its own goroutine.
func Test_accept(t *testing.T) {
	t.Run("a stalled upstream dial does not hold up later conns", func(t *testing.T) {
		// arrange: the first conn's dial blocks until released; the second conn's dial fails at once
		dialGate := newGate()
		t.Cleanup(dialGate.release)
		stalled := newStubConn(`{"stall":true}`)
		next := newStubConn("")
		hostCtx := newStubHostingContext()
		hostCtx.dialGate = dialGate
		listener := newStubListener(stalled, next)
		t.Cleanup(listener.close)

		// act
		go (&contextProvider{}).accept(listener, hostCtx)

		// assert
		requireClosed(t, dialGate.entered, "the first conn never reached its upstream dial")
		requireClosed(t, next.closed, "a conn accepted after a stalled dial was not handled while the dial was blocked")

		dialGate.release()
		requireClosed(t, stalled.closed, "the stalled conn was not closed after its dial failed")

		listener.close()
		requireClosed(t, hostCtx.closed, "the hosting context was not closed when the listener closed")
	})

	t.Run("a stalled accept completion does not hold up later conns", func(t *testing.T) {
		// arrange: the first conn dials its upstream, then blocks completing the accept until
		// released; the second conn's dial fails at once
		acceptGate := newGate()
		t.Cleanup(acceptGate.release)
		stalled := newStubConn(`{"stall":true}`)
		stalled.acceptGate = acceptGate
		next := newStubConn("")
		upstream, _ := net.Pipe()
		hostCtx := newStubHostingContext()
		hostCtx.upstream = upstream
		listener := newStubListener(stalled, next)
		t.Cleanup(listener.close)

		// act
		go (&contextProvider{}).accept(listener, hostCtx)

		// assert
		requireClosed(t, acceptGate.entered, "the first conn never reached its accept completion")
		requireClosed(t, next.closed, "a conn accepted after a stalled accept completion was not handled while it was blocked")

		acceptGate.release()
		requireClosed(t, stalled.closed, "the stalled conn was not closed after its accept completion failed")

		listener.close()
		requireClosed(t, hostCtx.closed, "the hosting context was not closed when the listener closed")
	})
}

// requireClosed fails the test unless ch is closed within a second.
func requireClosed(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		require.Fail(t, msg)
	}
}

// gate blocks callers of wait until release is called. entered is closed when the first caller
// reaches wait.
type gate struct {
	entered     chan struct{}
	enteredOnce sync.Once
	released    chan struct{}
	releaseOnce sync.Once
}

func newGate() *gate {
	return &gate{entered: make(chan struct{}), released: make(chan struct{})}
}

func (self *gate) wait() {
	self.enteredOnce.Do(func() { close(self.entered) })
	<-self.released
}

// release unblocks every current and later wait. It is safe to call more than once.
func (self *gate) release() {
	self.releaseOnce.Do(func() { close(self.released) })
}

// stubListener accepts its conns in order, then blocks until close, after which AcceptEdge fails.
type stubListener struct {
	edge.Listener
	conns     chan edge.Conn
	closeOnce sync.Once
}

func newStubListener(conns ...edge.Conn) *stubListener {
	result := &stubListener{conns: make(chan edge.Conn, len(conns))}
	for _, conn := range conns {
		result.conns <- conn
	}
	return result
}

func (self *stubListener) AcceptEdge() (edge.Conn, error) {
	if conn, ok := <-self.conns; ok {
		return conn, nil
	}
	return nil, errors.New("listener closed")
}

// close is safe to call more than once.
func (self *stubListener) close() {
	self.closeOnce.Do(func() { close(self.conns) })
}

// stubConn is an accepted conn whose accept completion always fails, after waiting on acceptGate
// when one is set. closed is closed by the first Close.
type stubConn struct {
	edge.Conn
	appData    []byte
	acceptGate *gate
	closeOnce  sync.Once
	closed     chan struct{}
}

func newStubConn(appData string) *stubConn {
	return &stubConn{appData: []byte(appData), closed: make(chan struct{})}
}

func (self *stubConn) GetAppData() []byte   { return self.appData }
func (self *stubConn) GetCircuitId() string { return "" }
func (self *stubConn) LocalAddr() net.Addr  { return nil }
func (self *stubConn) RemoteAddr() net.Addr { return nil }

func (self *stubConn) CompleteAcceptSuccess() error {
	if self.acceptGate != nil {
		self.acceptGate.wait()
	}
	return errors.New("circuit not started")
}

func (self *stubConn) CompleteAcceptFailed(error) {}

func (self *stubConn) Close() error {
	self.closeOnce.Do(func() { close(self.closed) })
	return nil
}

// stubHostingContext refuses dials whose options lack "stall". A stalled dial waits on dialGate and
// fails when one is set, and otherwise returns upstream. closed is closed by OnClose.
type stubHostingContext struct {
	HostingContext
	dialGate *gate
	upstream net.Conn
	closed   chan struct{}
}

func newStubHostingContext() *stubHostingContext {
	return &stubHostingContext{closed: make(chan struct{})}
}

func (self *stubHostingContext) ServiceName() string { return "test-service" }

func (self *stubHostingContext) Dial(options map[string]interface{}) (net.Conn, bool, error) {
	if options["stall"] != true {
		return nil, false, errors.New("connection refused")
	}
	if self.dialGate != nil {
		self.dialGate.wait()
		return nil, false, errors.New("i/o timeout")
	}
	return self.upstream, true, nil
}

func (self *stubHostingContext) OnClose() { close(self.closed) }
