package xgress_validation

import (
	"net"
	"time"
)

func NewBufferPipe(id string) *BufferPipe {
	a := NewBufferConn(id)
	b := NewBufferConn(id)
	return &BufferPipe{
		left:  &PipeEnd{id: id, reader: a, writer: b},
		right: &PipeEnd{id: id, reader: b, writer: a},
	}
}

type BufferPipe struct {
	left  *PipeEnd
	right *PipeEnd
}

func (self *BufferPipe) Left() net.Conn {
	return self.left
}

func (self *BufferPipe) Right() net.Conn {
	return self.right
}

type PipeEnd struct {
	id     string
	reader *BufferConn
	writer *BufferConn
}

func (self *PipeEnd) Read(b []byte) (n int, err error) {
	return self.reader.Read(b)
}

func (self *PipeEnd) Write(b []byte) (n int, err error) {
	return self.writer.Write(b)
}

func (self *PipeEnd) CloseRead() error {
	return self.reader.CloseRead()
}

func (self *PipeEnd) CloseWrite() error {
	return self.writer.CloseWrite()
}

func (self *PipeEnd) Close() error {
	if err := self.reader.Close(); err != nil {
		return err
	}
	return self.writer.Close()
}

func (self *PipeEnd) LocalAddr() net.Addr {
	return self.reader.LocalAddr()
}

func (self *PipeEnd) RemoteAddr() net.Addr {
	return self.reader.RemoteAddr()
}

func (self *PipeEnd) SetDeadline(t time.Time) error {
	if err := self.SetReadDeadline(t); err != nil {
		return err
	}
	return self.SetWriteDeadline(t)
}

func (self *PipeEnd) SetReadDeadline(t time.Time) error {
	return self.reader.SetReadDeadline(t)
}

func (self *PipeEnd) SetWriteDeadline(t time.Time) error {
	return self.writer.SetWriteDeadline(t)
}
