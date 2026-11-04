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

package xgress_edge

import (
	"testing"

	"github.com/openziti/channel/v5"
	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/router/state"
	"github.com/openziti/ziti/v2/router/xgress_common"
	"github.com/stretchr/testify/require"
)

// How an SDK ends its connection decides how the router tells the circuit: a connection that ended
// its data (a FIN) and then closed is a graceful end, and one that closed with no FIN before it, or
// was lost, is an abort. The router must not blur the two, since the far side can only tell the
// application behind it what the router told it.

func newEndTestConn(t *testing.T) (*edgeXgressConn, edge.SdkChannel) {
	mux := edge.NewChannelConnMapMux[*state.ConnState](nil)
	sdkChannel := edge.NewSingleSdkChannel(&NoopTestChannel{})
	conn := &edgeXgressConn{
		MsgChannel: *edge.NewEdgeMsgChannel(sdkChannel, 1),
		mux:        mux,
		seq:        NewMsgQueue(8),
	}
	require.NoError(t, mux.Add(conn))
	return conn, sdkChannel
}

func finDataMsg(connId uint32) *channel.Message {
	msg := edge.NewDataMsg(connId, nil)
	msg.PutUint32Header(edge.FlagsHeader, edge.FIN)
	return msg
}

func TestEdgeConnCloseWithoutFinIsAnAbort(t *testing.T) {
	req := require.New(t)
	conn, ch := newEndTestConn(t)

	conn.AcceptMessage(edge.NewStateClosedMsg(conn.Id(), "closed"), ch)

	_, _, err := conn.ReadPayload()
	req.ErrorIs(err, xgress.ErrPeerAborted)
	// an abort is still a close to anything that does not tell them apart
	req.ErrorIs(err, xgress.ErrPeerClosed)
}

func TestEdgeConnDataThenCloseWithoutFinIsAnAbort(t *testing.T) {
	req := require.New(t)
	conn, ch := newEndTestConn(t)

	conn.AcceptMessage(edge.NewDataMsg(conn.Id(), []byte("hello")), ch)
	conn.AcceptMessage(edge.NewStateClosedMsg(conn.Id(), "closed"), ch)

	data, _, err := conn.ReadPayload()
	req.NoError(err)
	req.Equal("hello", string(data))

	_, _, err = conn.ReadPayload()
	req.ErrorIs(err, xgress.ErrPeerAborted)
}

func TestEdgeConnCloseAfterFinIsGraceful(t *testing.T) {
	req := require.New(t)
	conn, ch := newEndTestConn(t)

	conn.AcceptMessage(finDataMsg(conn.Id()), ch)
	conn.AcceptMessage(edge.NewStateClosedMsg(conn.Id(), "closed"), ch)

	_, headers, err := conn.ReadPayload()
	req.NoError(err)
	req.Contains(headers, xgress_common.PayloadFlagsHeader, "the FIN travels on to the far side as it always has")

	_, _, err = conn.ReadPayload()
	req.ErrorIs(err, xgress.ErrPeerClosed)
	req.NotErrorIs(err, xgress.ErrPeerAborted)
}

// A connection lost with the SDK's channel never said anything: that is an abort.
func TestEdgeConnLostWithoutFinIsAnAbort(t *testing.T) {
	req := require.New(t)
	conn, _ := newEndTestConn(t)

	req.NoError(conn.HandleMuxClose())

	_, _, err := conn.ReadPayload()
	req.ErrorIs(err, xgress.ErrPeerAborted)
}

// ... unless it had ended its data first, which the far side already has.
func TestEdgeConnLostAfterFinIsGraceful(t *testing.T) {
	req := require.New(t)
	conn, ch := newEndTestConn(t)

	conn.AcceptMessage(finDataMsg(conn.Id()), ch)
	_, _, err := conn.ReadPayload()
	req.NoError(err)

	req.NoError(conn.HandleMuxClose())

	_, _, err = conn.ReadPayload()
	req.ErrorIs(err, xgress.ErrPeerClosed)
	req.NotErrorIs(err, xgress.ErrPeerAborted)
}
