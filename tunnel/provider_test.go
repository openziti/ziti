package tunnel

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/stretchr/testify/require"
)

func Test_accept_StalledDialDoesNotBlockLaterConns(t *testing.T) {
	testAcceptWithStalledConn(t, true)
}

func Test_accept_StalledAcceptCompletionDoesNotBlockLaterConns(t *testing.T) {
	testAcceptWithStalledConn(t, false)
}

// testAcceptWithStalledConn accepts a conn that stalls, in the upstream dial or in completing the
// accept, followed by one whose dial fails at once, and checks the second is handled while the
// first is still stalled.
func testAcceptWithStalledConn(t *testing.T, stallDial bool) {
	req := require.New(t)

	stallC := make(chan struct{})
	release := sync.OnceFunc(func() { close(stallC) })
	t.Cleanup(release)

	stalled := newStubConn(`{"stall":true}`)
	stalled.acceptSuccess = func() error {
		<-stallC
		return errors.New("circuit not started")
	}
	next := newStubConn("")

	hostCtx := &stubHostingContext{
		closed: make(chan struct{}),
		dial: func(options map[string]interface{}) (net.Conn, bool, error) {
			if options["stall"] != true {
				return nil, false, errors.New("connection refused")
			}
			if stallDial {
				<-stallC
				return nil, false, errors.New("i/o timeout")
			}
			upstream, peer := net.Pipe()
			t.Cleanup(func() { _ = peer.Close() })
			return upstream, true, nil
		},
	}

	listener := &stubListener{conns: make(chan edge.Conn, 2)}
	listener.conns <- stalled
	listener.conns <- next

	go (&contextProvider{}).accept(listener, hostCtx)

	awaitClosed(req, next.closed, "conn accepted after a stalled conn was not handled")
	release()
	awaitClosed(req, stalled.closed, "stalled conn was not closed once released")
	close(listener.conns)
	awaitClosed(req, hostCtx.closed, "hosting context was not closed when the listener closed")
}

func awaitClosed(req *require.Assertions, ch <-chan struct{}, msg string) {
	select {
	case <-ch:
	case <-time.After(time.Second):
		req.Fail(msg)
	}
}

type stubListener struct {
	edge.Listener
	conns chan edge.Conn
}

func (self *stubListener) AcceptEdge() (edge.Conn, error) {
	if conn, ok := <-self.conns; ok {
		return conn, nil
	}
	return nil, errors.New("listener closed")
}

type stubConn struct {
	edge.Conn
	appData       []byte
	acceptSuccess func() error
	closeOnce     sync.Once
	closed        chan struct{}
}

func newStubConn(appData string) *stubConn {
	return &stubConn{appData: []byte(appData), closed: make(chan struct{})}
}

func (self *stubConn) GetAppData() []byte           { return self.appData }
func (self *stubConn) GetCircuitId() string         { return "" }
func (self *stubConn) LocalAddr() net.Addr          { return nil }
func (self *stubConn) RemoteAddr() net.Addr         { return nil }
func (self *stubConn) CompleteAcceptSuccess() error { return self.acceptSuccess() }
func (self *stubConn) CompleteAcceptFailed(_ error) {}

func (self *stubConn) Close() error {
	self.closeOnce.Do(func() { close(self.closed) })
	return nil
}

type stubHostingContext struct {
	HostingContext
	dial   func(options map[string]interface{}) (net.Conn, bool, error)
	closed chan struct{}
}

func (self *stubHostingContext) ServiceName() string { return "test-service" }

func (self *stubHostingContext) Dial(options map[string]interface{}) (net.Conn, bool, error) {
	return self.dial(options)
}

func (self *stubHostingContext) OnClose() { close(self.closed) }
