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

package mesh

import (
	"github.com/openziti/channel/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"sync/atomic"
	"time"
)

func newRaftPeerConn(peer *Peer, localAddr net.Addr) *raftPeerConn {
	return &raftPeerConn{
		peer:        peer,
		localAddr:   localAddr,
		readTimeout: newDeadline(),
		readC:       make(chan []byte, 16),
		closeNotify: make(chan struct{}),
	}
}

// raftPeerConn presents a net.Conn API over a channel. This allows us to multiplex raft traffic as well
// as our own traffic (such as command forwarding) over the same network connection
type raftPeerConn struct {
	peer          *Peer
	localAddr     net.Addr
	readTimeout   *deadline
	writeDeadline time.Time
	readC         chan []byte
	leftOver      []byte
	closeNotify   chan struct{}
	closed        atomic.Bool
}

func (self *raftPeerConn) ContentType() int32 {
	return RaftDataType
}

func (self *raftPeerConn) HandleReceive(m *channel.Message, _ channel.Channel) {
	select {
	case self.readC <- m.Body:
		//logrus.Infof("received %v bytes from raft peer %v", len(m.Body), self.peer.Id)
	case <-self.closeNotify:
		logrus.Errorf("received data for closed channel from %v at %v", self.peer.Id, self.peer.Address)
	}
}

func (self *raftPeerConn) Read(b []byte) (n int, err error) {
	leftOverLen := len(self.leftOver)
	if leftOverLen > 0 {
		copy(b, self.leftOver)
		if leftOverLen > len(b) {
			self.leftOver = self.leftOver[len(b):]
			//logrus.Infof("raft read return %v bytes from leftover", len(b))
			return len(b), nil
		}

		self.leftOver = nil
		//logrus.Infof("raft read return %v bytes from leftover", leftOverLen)
		return leftOverLen, nil
	}

	var data []byte

	select {
	case data = <-self.readC: // get any data that's waiting. If none is waiting, try again with checks for deadline/close
	default:
		select {
		case data = <-self.readC:
		case <-self.readTimeout.C:
			//logrus.Info("raft read returned with deadline")
			return 0, errors.New("timeout")
		case <-self.closeNotify:
			//logrus.Info("raft read returned with EOF after close")
			return 0, io.EOF
		}
	}

	if data == nil {
		//logrus.Info("raft read returned with EOF")
		return 0, io.EOF
	}

	dataLen := len(data)
	copy(b, data)
	if dataLen <= len(b) {
		//logrus.Infof("raft read returned with %v bytes", dataLen)
		return dataLen, nil
	}

	//logrus.Infof("storing %v leftover bytes", len(self.leftOver))
	self.leftOver = data[len(b):]

	// logrus.Infof("raft read returned with %v bytes", len(b))
	return len(b), nil
}

func (self *raftPeerConn) Write(b []byte) (n int, err error) {
	if self.closed.Load() {
		return 0, errors.New("connection closed")
	}
	// logrus.Infof("writing %v bytes to raft peer %v", len(b), self.peer.Id)
	msg := channel.NewMessage(RaftDataType, b)
	if deadline := self.writeDeadline; !deadline.IsZero() {
		now := time.Now()
		if deadline.After(now) {
			return len(b), msg.WithTimeout(deadline.Sub(now)).SendAndWaitForWire(self.peer.Channel)
		}
		return 0, errors.New("timeout")
	}
	return len(b), msg.Send(self.peer.Channel)
}

func (self *raftPeerConn) Close() error {
	return self.peer.closeRaftConn(5 * time.Second)
}

func (self *raftPeerConn) close() bool {
	if self.closed.CompareAndSwap(false, true) {
		close(self.closeNotify)
		return true
	}
	return false
}

func (self *raftPeerConn) LocalAddr() net.Addr {
	return self.localAddr
}

func (self *raftPeerConn) RemoteAddr() net.Addr {
	return &meshAddr{
		network: "mesh",
		addr:    string(self.peer.Address),
	}
}

func (self *raftPeerConn) SetDeadline(t time.Time) error {
	now := time.Now()
	if t.After(now) {
		duration := t.Sub(now)
		self.readTimeout.SetTimeout(duration)
		self.writeDeadline = t
	} else {
		self.readTimeout.Trigger()
		self.writeDeadline = time.Time{}
	}
	return nil
}

func (self *raftPeerConn) SetReadDeadline(t time.Time) error {
	now := time.Now()
	if t.After(now) {
		duration := t.Sub(now)
		self.readTimeout.SetTimeout(duration)
	} else {
		self.readTimeout.Trigger()
	}
	return nil
}

func (self *raftPeerConn) SetWriteDeadline(t time.Time) error {
	self.writeDeadline = t
	return nil
}
