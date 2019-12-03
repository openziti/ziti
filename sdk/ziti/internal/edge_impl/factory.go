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

package edge_impl

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-edge/sdk/ziti/edge"
	"github.com/netfoundry/ziti-foundation/util/sequencer"
)

const (
	// TODO: Add configuration mechanism for the SDK
	DefaultMaxOutOfOrderMsgs = 5000
)

type connFactory struct {
	ch     channel2.Channel
	msgMux *edge.MsgMux
}

func NewEdgeConnFactory(ch channel2.Channel) edge.ConnFactory {
	connFactory := &connFactory{
		ch:     ch,
		msgMux: edge.NewMsgMux(),
	}

	ch.AddReceiveHandler(&edge.FunctionReceiveAdapter{
		Type:    edge.ContentTypeDial,
		Handler: connFactory.msgMux.HandleReceive,
	})

	ch.AddReceiveHandler(&edge.FunctionReceiveAdapter{
		Type:    edge.ContentTypeStateClosed,
		Handler: connFactory.msgMux.HandleReceive,
	})

	// Since data is the common message type, it gets to be dispatched directly
	ch.AddReceiveHandler(connFactory.msgMux)
	ch.AddCloseHandler(connFactory.msgMux)

	return connFactory
}

func (conn *connFactory) NewConn(service string) edge.Conn {
	id := connSeq.Next()

	edgeCh := &edgeConn{
		MsgChannel: *edge.NewEdgeMsgChannel(conn.ch, id),
		readQ:      sequencer.NewSingleWriterSeq(DefaultMaxOutOfOrderMsgs),
		msgMux:     conn.msgMux,
		serviceId:  service,
	}

	_ = conn.msgMux.AddMsgSink(edgeCh) // duplicate errors only happen on the server side, since client controls ids
	return edgeCh
}

func (conn *connFactory) Close() error {
	return conn.ch.Close()
}

func (conn *connFactory) IsClosed() bool {
	return conn.ch.IsClosed()
}