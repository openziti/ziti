package xgress

import (
	"encoding/binary"
	"github.com/openziti/channel/v2"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/metrics/metrics_pb"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
	"time"
)

type testConn struct {
	ch          chan uint64
	closeNotify chan struct{}
	closed      concurrenz.AtomicBoolean
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

type noopForwarder struct{}

func (n noopForwarder) ForwardPayload(srcAddr Address, payload *Payload) error {
	return nil
}

func (n noopForwarder) ForwardAcknowledgement(srcAddr Address, acknowledgement *Acknowledgement) error {
	return nil
}

type noopReceiveHandler struct{}

func (n noopReceiveHandler) HandleXgressReceive(payload *Payload, x *Xgress) {}

func (n noopReceiveHandler) HandleControlReceive(*Control, *Xgress) {}

func Test_Ordering(t *testing.T) {
	closeNotify := make(chan struct{})
	metricsRegistry := metrics.NewUsageRegistry("test", map[string]string{}, closeNotify)
	InitPayloadIngester(closeNotify)
	InitMetrics(metricsRegistry)
	InitAcker(&noopForwarder{}, metricsRegistry, closeNotify)

	conn := &testConn{
		ch:          make(chan uint64, 1),
		closeNotify: make(chan struct{}),
	}

	x := NewXgress(&identity.TokenId{Token: "test"}, "test", conn, Initiator, DefaultOptions(), nil)
	x.receiveHandler = noopReceiveHandler{}
	go x.tx()

	defer x.Close()

	msgCount := 100000

	errorCh := make(chan error, 1)

	go func() {
		for i := 0; i < msgCount; i++ {
			data := make([]byte, 8)
			binary.LittleEndian.PutUint64(data, uint64(i))
			payload := &Payload{
				Header: Header{
					CircuitId:      "test",
					Flags:          SetOriginatorFlag(0, Terminator),
					RecvBufferSize: 16000,
					RTT:            0,
				},
				Sequence: int32(i),
				Headers:  nil,
				Data:     data,
			}
			if err := x.SendPayload(payload); err != nil {
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

type noopMetricsHandler struct{}

func (n noopMetricsHandler) AcceptMetrics(message *metrics_pb.MetricsMessage) {
}
