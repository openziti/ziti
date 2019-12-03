/*
	Copyright 2019 Netfoundry, Inc.

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

package edge

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/netfoundry/ziti-foundation/transport/tls"
	"github.com/netfoundry/ziti-foundation/util/sequence"
	"errors"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

type addrParser struct {
	p tls.AddressParser
}

func (ap addrParser) Parse(addressString string) (transport.Address, error) {
	return ap.p.Parse(strings.Replace(addressString, "/", "", -1))
}

func init() {
	transport.AddAddressParser(new(addrParser))
}

type ConnFactory interface {
	io.Closer
	IsClosed() bool
	NewConn(service string) Conn
}

type Identifiable interface {
	Id() uint32
}

type Conn interface {
	net.Conn
	Identifiable
	NewConn(service string) Conn
	Connect(session *NetworkSession) (net.Conn, error)
	Listen(session *NetworkSession, serviceName string) (net.Listener, error)
	IsClosed() bool
}

type MsgChannel struct {
	channel2.Channel
	id            uint32
	msgIdSeq      *sequence.Sequence
	writeDeadline time.Time
	trace         bool
}

func NewEdgeMsgChannel(ch channel2.Channel, connId uint32) *MsgChannel {
	traceEnabled := strings.EqualFold("true", os.Getenv("ZITI_TRACE_ENABLED"))
	if traceEnabled {
		pfxlog.Logger().Info("Ziti message tracing ENABLED")
	}

	return &MsgChannel{
		Channel:  ch,
		id:       connId,
		msgIdSeq: sequence.NewSequence(),
		trace:    traceEnabled,
	}
}

func (ec *MsgChannel) Id() uint32 {
	return ec.id
}

func (ec *MsgChannel) SetWriteDeadline(t time.Time) error {
	ec.writeDeadline = t
	return nil
}

func (ec *MsgChannel) Write(data []byte) (n int, err error) {
	return ec.WriteTraced(data, nil)
}

func (ec *MsgChannel) WriteTraced(data []byte, msgUUID []byte) (int, error) {
	msg := NewDataMsg(ec.id, ec.msgIdSeq.Next(), data)
	if msgUUID != nil {
		msg.Headers[UUIDHeader] = msgUUID
	}
	ec.TraceMsg("write", msg)
	pfxlog.Logger().WithFields(GetLoggerFields(msg)).Debugf("writing %v bytes", len(data))

	// NOTE: We need to wait for the buffer to be on the wire before returning. The Writer contract
	//       states that buffers are not allowed be retained, and if we have it queued asynchronously
	//       it is retained and we can cause data corruption
	var err error
	if ec.writeDeadline.IsZero() {
		var errC chan error
		errC, err = ec.Channel.SendAndSync(msg)
		if err == nil {
			err = <-errC
		}
	} else {
		err = ec.Channel.SendWithTimeout(msg, time.Until(ec.writeDeadline))
	}

	if err != nil {
		return 0, err
	}

	return len(data), nil
}

func (ec *MsgChannel) SendState(msg *channel2.Message) error {
	msg.PutUint32Header(SeqHeader, ec.msgIdSeq.Next())
	ec.TraceMsg("SendState", msg)
	syncC, err := ec.SendAndSyncWithPriority(msg, channel2.High)
	if err != nil {
		return err
	}

	select {
	case err = <-syncC:
		return err
	case <-time.After(time.Second * 5):
		return errors.New("timed out waiting for close message send to complete")
	}
}

func (ec *MsgChannel) TraceMsg(source string, msg *channel2.Message) {
	msgUUID, found := msg.Headers[UUIDHeader]
	if ec.trace && !found {
		newUUID, err := uuid.NewRandom()
		if err == nil {
			msgUUID = newUUID[:]
			msg.Headers[UUIDHeader] = msgUUID
		} else {
			pfxlog.Logger().WithField("connId", ec.id).WithError(err).Infof("failed to create trace uuid")
		}
	}

	if msgUUID != nil {
		pfxlog.Logger().WithFields(GetLoggerFields(msg)).WithField("source", source).Debug("tracing message")
	}
}