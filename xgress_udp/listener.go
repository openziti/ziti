package xgress_udp

import "net"

type Listener interface {
	WriteTo(data []byte, addr net.Addr) (int, error)
	GetSession(sessionId string) (Session, bool)
	DeleteSession(sessionId string)
	QueueEvent(event EventHandler)
	LogContext() string
}

type Session interface {
	State() SessionState
	SetState(state SessionState)
	Address() net.Addr
	ReadPayload() ([]byte, map[uint8][]byte, error)
	Write(data []byte) (n int, err error)
	WritePayload(data []byte, headers map[uint8][]byte) (n int, err error)
	QueueRead(data []byte)
	Close() error
	LogContext() string
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
