package xgress

import (
	"encoding/binary"
	"github.com/openziti/channel/v4"
	"github.com/stretchr/testify/require"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

type testConn struct {
	ch          chan uint64
	closeNotify chan struct{}
	closed      atomic.Bool
}

func (conn *testConn) Close() error {
	if conn.closed.CompareAndSwap(false, true) {
		close(conn.closeNotify)
	}
	return nil
}

func (conn *testConn) LogContext() string {
	return "test"
}

func (conn *testConn) ReadPayload() ([]byte, map[uint8][]byte, error) {
	<-conn.closeNotify
	return nil, nil, io.EOF
}

func (conn *testConn) WritePayload(bytes []byte, _ map[uint8][]byte) (int, error) {
	val := binary.LittleEndian.Uint64(bytes)
	conn.ch <- val
	return len(bytes), nil
}

func (conn *testConn) HandleControlMsg(ControlType, channel.Headers, ControlReceiver) error {
	return nil
}

type noopReceiveHandler struct {
	payloadIngester *PayloadIngester
}

func (n noopReceiveHandler) RetransmitPayload(srcAddr Address, payload *Payload) error {
	return nil
}

func (n noopReceiveHandler) GetMetrics() Metrics {
	return noopMetrics{}
}

func (n noopReceiveHandler) GetRetransmitter() *Retransmitter {
	return nil
}

func (n noopReceiveHandler) GetPayloadIngester() *PayloadIngester {
	return n.payloadIngester
}

func (n noopReceiveHandler) ForwardAcknowledgement(*Acknowledgement, Address) {}

func (n noopReceiveHandler) ForwardPayload(*Payload, *Xgress) {}

func (n noopReceiveHandler) ForwardControlMessage(*Control, *Xgress) {}

func Test_Ordering(t *testing.T) {
	closeNotify := make(chan struct{})

	conn := &testConn{
		ch:          make(chan uint64, 1),
		closeNotify: make(chan struct{}),
	}

	x := NewXgress("test", "ctrl", "test", conn, Initiator, DefaultOptions(), nil)
	x.dataPlane = noopReceiveHandler{
		payloadIngester: NewPayloadIngester(closeNotify),
	}
	go x.tx()

	defer x.Close()

	msgCount := 100000

	errorCh := make(chan error, 1)

	go func() {
		for i := 0; i < msgCount; i++ {
			data := make([]byte, 8)
			binary.LittleEndian.PutUint64(data, uint64(i))
			payload := &Payload{
				CircuitId: "test",
				Flags:     SetOriginatorFlag(0, Terminator),
				RTT:       0,
				Sequence:  int32(i),
				Headers:   nil,
				Data:      data,
			}
			if err := x.SendPayload(payload, 0, PayloadTypeXg); err != nil {
				errorCh <- err
				x.Close()
				return
			}
		}
	}()

	timeout := time.After(20 * time.Second)

	req := require.New(t)
	for i := 0; i < msgCount; i++ {
		select {
		case next := <-conn.ch:
			req.Equal(uint64(i), next)
		case <-conn.closeNotify:
			req.Fail("test failed with count at %v", i)
		case err := <-errorCh:
			req.NoError(err)
		case <-timeout:
			req.Failf("timed out", "count at %v", i)
		}
	}
}

type noopMetrics struct{}

func (n noopMetrics) MarkAckReceived() {}

func (n noopMetrics) MarkPayloadDropped() {}

func (n noopMetrics) MarkDuplicateAck() {}

func (n noopMetrics) MarkDuplicatePayload() {}

func (n noopMetrics) BufferBlockedByLocalWindow() {}

func (n noopMetrics) BufferUnblockedByLocalWindow() {}

func (n noopMetrics) BufferBlockedByRemoteWindow() {}

func (n noopMetrics) BufferUnblockedByRemoteWindow() {}

func (n noopMetrics) PayloadWritten(time.Duration) {}

func (n noopMetrics) BufferUnblocked(time.Duration) {}

func (n noopMetrics) SendPayloadBuffered(int64) {}

func (n noopMetrics) SendPayloadDelivered(int64) {}
