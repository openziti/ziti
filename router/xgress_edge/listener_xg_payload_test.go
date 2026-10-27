package xgress_edge

import (
	"testing"

	"github.com/openziti/channel/v5"
	"github.com/openziti/sdk-golang/v2/xgress"
	sdkedge "github.com/openziti/sdk-golang/v2/ziti/edge"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

func newXgPayloadTestConn(ch *recordingTestChannel) *edgeClientConn {
	return &edgeClientConn{
		ch:         sdkedge.NewSingleSdkChannel(ch),
		xgCircuits: cmap.New[*xgEdgeForwarder](),
	}
}

// sdkPayloadMsg builds a payload an SDK initiator sends on conn connId for circuitId.
func sdkPayloadMsg(connId uint32, circuitId string, flags xgress.Flag) *channel.Message {
	payload := &xgress.Payload{
		CircuitId: circuitId,
		Flags:     xgress.SetOriginatorFlag(uint32(flags), xgress.Initiator),
		Sequence:  5,
		Data:      []byte("data"),
	}
	msg := payload.Marshall()
	msg.PutUint32Header(sdkedge.ConnIdHeader, connId)
	return msg
}

// A payload for a circuit the router doesn't have is answered with a write-failed payload on the
// same conn, from the other end of the circuit, so the SDK closes its send buffer.
func Test_XgPayloadForUnknownCircuitIsAnsweredWithWriteFailed(t *testing.T) {
	req := require.New(t)
	testCh := &recordingTestChannel{}
	conn := newXgPayloadTestConn(testCh)

	conn.handleXgPayload(sdkPayloadMsg(7, "gone-circuit", 0), nil)

	req.Len(testCh.sent, 1)
	msg, ok := testCh.sent[0].(*channel.Message)
	req.True(ok)
	connId, found := msg.GetUint32Header(sdkedge.ConnIdHeader)
	req.True(found)
	req.Equal(uint32(7), connId)

	reply, err := xgress.UnmarshallPayload(msg)
	req.NoError(err)
	req.Equal("gone-circuit", reply.CircuitId)
	req.True(reply.IsFlagWriteFailedSet())
	req.Equal(xgress.Terminator, reply.GetOriginator())
	req.Empty(reply.Data)
}

// An SDK that is already ending the circuit, or is itself reporting a failed write, gets no reply.
func Test_XgPayloadForUnknownCircuitIsNotAnsweredWhenEndingOrFailed(t *testing.T) {
	for name, flag := range map[string]xgress.Flag{
		"end of circuit": xgress.PayloadFlagCircuitEnd,
		"write failed":   xgress.PayloadFlagWriteFailed,
	} {
		t.Run(name, func(t *testing.T) {
			testCh := &recordingTestChannel{}
			conn := newXgPayloadTestConn(testCh)

			conn.handleXgPayload(sdkPayloadMsg(7, "gone-circuit", flag), nil)

			require.Empty(t, testCh.sent)
		})
	}
}
