package xgress_validation

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"io"
	"net"
	"os"
	"sync/atomic"
	"time"
)

func NewBufferConn(id string) *BufferConn {
	return &BufferConn{
		id:                  id,
		ch:                  make(chan []byte, 10),
		readCloseNotify:     make(chan struct{}),
		readDeadlineChange:  make(chan struct{}),
		writeCloseNotify:    make(chan struct{}),
		writeDeadlineChange: make(chan struct{}),
	}
}

type BufferConn struct {
	id       string
	ch       chan []byte
	leftover []byte

	readClosed         atomic.Bool
	readCloseNotify    chan struct{}
	readDeadline       concurrenz.AtomicValue[time.Time]
	readDeadlineChange chan struct{}

	writeClosed         atomic.Bool
	writeCloseNotify    chan struct{}
	writeDeadline       concurrenz.AtomicValue[time.Time]
	writeDeadlineChange chan struct{}
}

func (self *BufferConn) Read(b []byte) (n int, err error) {
	if len(self.leftover) > 0 {
		n = copy(b, self.leftover)
		if n == len(self.leftover) {
			self.leftover = nil
		} else {
			self.leftover = self.leftover[n:]
		}
		return n, nil
	}

	for {
		var deadlineC <-chan time.Time
		deadline := self.readDeadline.Load()
		if !deadline.IsZero() {
			deadlineC = time.After(time.Until(deadline))
		}

		select {
		case <-deadlineC:
			return 0, os.ErrDeadlineExceeded
		case <-self.readCloseNotify:
			pfxlog.Logger().Info("read closed, return EOF")
			return 0, io.EOF
		case <-self.readDeadlineChange:
			// loop around and retry with an updated read deadline
		case data := <-self.ch:
			n = copy(b, data)

			if n < len(data) {
				self.leftover = data[n:]
			}
			return n, nil
		}
	}
}

func (self *BufferConn) Write(b []byte) (n int, err error) {
	for {
		var deadlineC <-chan time.Time
		deadline := self.writeDeadline.Load()
		if !deadline.IsZero() {
			deadlineC = time.After(time.Until(deadline))
		}

		select {
		case self.ch <- b:
			return len(b), nil
		case <-deadlineC:
			return 0, os.ErrDeadlineExceeded
		case <-self.writeCloseNotify:
			return 0, io.EOF
		case <-self.writeDeadlineChange:
			// loop around and retry with an updated write deadline
		}
	}
}

func (self *BufferConn) CloseWrite() error {
	if self.writeClosed.CompareAndSwap(false, true) {
		close(self.writeCloseNotify)
	}
	return nil
}

func (self *BufferConn) CloseRead() error {
	if self.readClosed.CompareAndSwap(false, true) {
		close(self.readCloseNotify)
	}
	return nil
}

func (self *BufferConn) Close() error {
	_ = self.CloseRead()
	_ = self.CloseWrite()
	return nil
}

func (self *BufferConn) LocalAddr() net.Addr {
	return testAddr("127.0.0.1:1234")
}

func (self *BufferConn) RemoteAddr() net.Addr {
	return testAddr("127.0.0.1:4321")
}

func (self *BufferConn) SetDeadline(t time.Time) error {
	if err := self.SetWriteDeadline(t); err != nil {
		return err
	}
	return self.SetReadDeadline(t)
}

func (self *BufferConn) SetReadDeadline(t time.Time) error {
	self.readDeadline.Store(t)
	select {
	case self.readDeadlineChange <- struct{}{}:
	default:
	}
	return nil
}

func (self *BufferConn) SetWriteDeadline(t time.Time) error {
	self.writeDeadline.Store(t)
	select {
	case self.writeDeadlineChange <- struct{}{}:
	default:
	}
	return nil
}

type testAddr string

func (t testAddr) Network() string {
	return "tcp"
}

func (t testAddr) String() string {
	return string(t)
}
