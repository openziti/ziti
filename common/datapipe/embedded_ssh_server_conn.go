package datapipe

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/foundation/v2/concurrenz"
	"io"
	"net"
	"sync/atomic"
	"time"
)

type MessageTypes struct {
	DataMessageType  int32
	PipeIdHeaderType int32
	CloseMessageType int32
}

func NewEmbeddedSshConn(ch channel.Channel, id uint32, msgTypes *MessageTypes) *EmbeddedSshConn {
	return &EmbeddedSshConn{
		id:          id,
		ch:          ch,
		ReadAdapter: channel.NewReadAdapter(fmt.Sprintf("pipe-%d", id), 4),
		msgTypes:    msgTypes,
	}
}

type EmbeddedSshConn struct {
	msgTypes *MessageTypes
	id       uint32
	ch       channel.Channel
	closed   atomic.Bool
	*channel.ReadAdapter
	sshConn  concurrenz.AtomicValue[io.Closer]
	deadline atomic.Pointer[time.Time]
}

func (self *EmbeddedSshConn) Id() uint32 {
	return self.id
}

func (self *EmbeddedSshConn) SetSshConn(conn io.Closer) {
	self.sshConn.Store(conn)
}

func (self *EmbeddedSshConn) WriteToServer(data []byte) error {
	return self.ReadAdapter.PushData(data)
}

func (self *EmbeddedSshConn) Write(data []byte) (n int, err error) {
	msg := channel.NewMessage(self.msgTypes.DataMessageType, data)
	msg.PutUint32Header(self.msgTypes.PipeIdHeaderType, self.id)
	deadline := time.Second
	if val := self.deadline.Load(); val != nil && !val.IsZero() {
		deadline = time.Until(*val)
	}
	return len(data), msg.WithTimeout(deadline).SendAndWaitForWire(self.ch)
}

func (self *EmbeddedSshConn) Close() error {
	self.CloseWithErr(errors.New("close called"))
	return nil
}

func (self *EmbeddedSshConn) CloseWithErr(err error) {
	if self.closed.CompareAndSwap(false, true) {
		self.ReadAdapter.Close()
		log := pfxlog.ContextLogger(self.ch.Label()).WithField("connId", self.id)

		log.WithError(err).Info("closing mgmt pipe connection")

		if sshConn := self.sshConn.Load(); sshConn != nil {
			if closeErr := sshConn.Close(); closeErr != nil {
				log.WithError(closeErr).Error("failed closing mgmt pipe embedded ssh connection")
			}
		}

		if !self.ch.IsClosed() && err != io.EOF && err != nil {
			msg := channel.NewMessage(self.msgTypes.CloseMessageType, []byte(err.Error()))
			msg.PutUint32Header(self.msgTypes.PipeIdHeaderType, self.id)
			if sendErr := self.ch.Send(msg); sendErr != nil {
				log.WithError(sendErr).Error("failed sending mgmt pipe close message")
			}
		}

		if closeErr := self.ch.Close(); closeErr != nil {
			log.WithError(closeErr).Error("failed closing mgmt pipe client channel")
		}
	}
}

func (self *EmbeddedSshConn) LocalAddr() net.Addr {
	return embeddedSshPipeAddr{
		id: self.id,
	}
}

func (self *EmbeddedSshConn) RemoteAddr() net.Addr {
	return embeddedSshPipeAddr{
		id: self.id,
	}
}

func (self *EmbeddedSshConn) SetDeadline(t time.Time) error {
	if err := self.ReadAdapter.SetReadDeadline(t); err != nil {
		return err
	}
	return self.SetWriteDeadline(t)
}

func (self *EmbeddedSshConn) SetWriteDeadline(t time.Time) error {
	self.deadline.Store(&t)
	return nil
}

func (self *EmbeddedSshConn) WriteToClient(data []byte) error {
	_, err := self.Write(data)
	return err
}

type embeddedSshPipeAddr struct {
	id uint32
}

func (self embeddedSshPipeAddr) Network() string {
	return "ziti"
}

func (self embeddedSshPipeAddr) String() string {
	return fmt.Sprintf("ssh-pipe-%d", self.id)
}
