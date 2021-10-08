/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/fabric/router/xgress"
	"io"
	"net"
)

type Listener interface {
	io.Closer
	WriteTo(data []byte, addr net.Addr) (int, error)
	GetSession(sessionId string) (Session, bool)
	DeleteSession(sessionId string)
	QueueEvent(event EventHandler)
	LogContext() string
}

type Session interface {
	xgress.Connection
	State() SessionState
	SetState(state SessionState)
	Address() net.Addr
	Write(data []byte) (n int, err error)
	QueueRead(data []byte)
	TimeoutNanos() int64
	MarkActivity()
	SessionId() string
}

type EventHandler interface {
	Handle(listener Listener)
}

type SessionState uint8

const (
	SessionStateNew SessionState = iota
	SessionStateEstablished
)
