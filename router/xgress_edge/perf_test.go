package xgress_edge

import (
	"crypto/x509"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/handler_xgress"
	metrics2 "github.com/openziti/fabric/router/metrics"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type noopMetricsHandler struct{}

func (n noopMetricsHandler) AcceptMetrics(*metrics_pb.MetricsMessage) {
}

func newMirrorLink(fwd *forwarder.Forwarder) *mirrorLink {
	result := &mirrorLink{
		fwd:  fwd,
		acks: make(chan *xgress.Acknowledgement, 4),
	}
	go result.run()
	return result
}

type mirrorLink struct {
	fwd  *forwarder.Forwarder
	acks chan *xgress.Acknowledgement
}

func (link *mirrorLink) Inspect() map[string]interface{} {
	return nil
}

func (link *mirrorLink) CloseNotified() error {
	return nil
}

func (link *mirrorLink) DestVersion() string {
	return "0.0.0"
}

func (link *mirrorLink) LinkProtocol() string {
	return "tls"
}

func (link *mirrorLink) HandleCloseNotification(f func()) {
	f()
}

func (link *mirrorLink) DestinationId() string {
	return "test"
}

func (link *mirrorLink) Id() *identity.TokenId {
	return &identity.TokenId{Token: "router1"}
}

func (link *mirrorLink) SendPayload(payload *xgress.Payload) error {
	ack := &xgress.Acknowledgement{
		Header: xgress.Header{
			CircuitId:      "test",
			Flags:          0,
			RecvBufferSize: 0,
			RTT:            payload.RTT,
		},
	}
	ack.Sequence = append(ack.Sequence, payload.Sequence)
	link.acks <- ack
	return nil
}

func (link *mirrorLink) run() {
	for ack := range link.acks {
		err := link.fwd.ForwardAcknowledgement("router1", ack)
		if err != nil {
			pfxlog.Logger().WithError(err).Infof("unable to forward ack")
		}
	}
}

func (link *mirrorLink) SendAcknowledgement(*xgress.Acknowledgement) error {
	return nil
}

func (link *mirrorLink) SendControl(*xgress.Control) error {
	return nil
}

func (link *mirrorLink) Close() error {
	panic("implement me")
}

func Benchmark_ChMsgMuxWritePerf(b *testing.B) {
	mux := edge.NewChMsgMux()
	writePerf(b, mux)
}

func Benchmark_CowMapWritePerf(b *testing.B) {
	mux := edge.NewCowMapMsgMux()
	writePerf(b, mux)
}

func writePerf(b *testing.B, mux edge.MsgMux) {
	testChannel := &NoopTestChannel{}

	listener := &listener{}

	proxy := &edgeClientConn{
		msgMux:       mux,
		listener:     listener,
		fingerprints: nil,
		ch:           testChannel,
	}

	conn := &edgeXgressConn{
		MsgChannel: *edge.NewEdgeMsgChannel(proxy.ch, 1),
		seq:        NewMsgQueue(4),
	}

	req := require.New(b)
	req.NoError(mux.AddMsgSink(conn))

	metricsRegistry := metrics.NewUsageRegistry("test", map[string]string{}, nil)
	xgress.InitMetrics(metricsRegistry)

	fwdOptions := forwarder.DefaultOptions()
	fwd := forwarder.NewForwarder(metricsRegistry, nil, nil, fwdOptions, nil)

	link := newMirrorLink(fwd)

	fwd.RegisterLink(link)
	fwd.Route(&ctrl_pb.Route{
		CircuitId: "test",
		Egress:    nil,
		Forwards: []*ctrl_pb.Route_Forward{
			{SrcAddress: "test", DstAddress: "router1"},
			{SrcAddress: "router1", DstAddress: "test"},
		},
	})

	x := xgress.NewXgress(&identity.TokenId{Token: "test"}, "test", conn, xgress.Initiator, xgress.DefaultOptions())
	x.SetReceiveHandler(handler_xgress.NewReceiveHandler(fwd))
	x.AddPeekHandler(metrics2.NewXgressPeekHandler(fwd.MetricsRegistry()))

	//x.SetCloseHandler(bindHandler.closeHandler)
	fwd.RegisterDestination(x.CircuitId(), x.Address(), x)

	x.Start()

	b.ReportAllocs()
	b.ResetTimer()

	data := make([]byte, 1024)

	for i := 0; i < b.N; i++ {
		msg := edge.NewDataMsg(1, uint32(i+1), data)
		mux.HandleReceive(msg, testChannel)
		b.SetBytes(1024)
	}
}

type simpleTestXgConn struct {
	ch chan []byte
}

func (conn *simpleTestXgConn) write(data []byte) {
	conn.ch <- data
}

func (conn *simpleTestXgConn) Close() error {
	panic("implement me")
}

func (conn *simpleTestXgConn) LogContext() string {
	return "test"
}

func (conn *simpleTestXgConn) ReadPayload() ([]byte, map[uint8][]byte, error) {
	result := <-conn.ch
	return result, nil, nil
}

func (conn *simpleTestXgConn) WritePayload([]byte, map[uint8][]byte) (int, error) {
	panic("implement me")
}

func (conn *simpleTestXgConn) HandleControlMsg(xgress.ControlType, channel.Headers, xgress.ControlReceiver) error {
	return nil
}

func Benchmark_BaselinePerf(b *testing.B) {
	conn := &simpleTestXgConn{
		ch: make(chan []byte),
	}
	xgOptions := xgress.DefaultOptions()

	metricsRegistry := metrics.NewUsageRegistry("test", map[string]string{}, nil)
	xgress.InitMetrics(metricsRegistry)

	fwdOptions := forwarder.DefaultOptions()
	fwd := forwarder.NewForwarder(metricsRegistry, nil, nil, fwdOptions, nil)

	link := newMirrorLink(fwd)

	fwd.RegisterLink(link)
	fwd.Route(&ctrl_pb.Route{
		CircuitId: "test",
		Egress:    nil,
		Forwards: []*ctrl_pb.Route_Forward{
			{SrcAddress: "test", DstAddress: "router1"},
			{SrcAddress: "router1", DstAddress: "test"},
		},
	})

	x := xgress.NewXgress(&identity.TokenId{Token: "test"}, "test", conn, xgress.Initiator, xgOptions)
	x.SetReceiveHandler(handler_xgress.NewReceiveHandler(fwd))
	x.AddPeekHandler(metrics2.NewXgressPeekHandler(fwd.MetricsRegistry()))

	//x.SetCloseHandler(bindHandler.closeHandler)
	fwd.RegisterDestination(x.CircuitId(), x.Address(), x)

	x.Start()

	b.ReportAllocs()
	b.ResetTimer()

	data := make([]byte, 1024)

	for i := 0; i < b.N; i++ {
		conn.write(data)
		b.SetBytes(1024)
	}
}

type NoopTestChannel struct {
}

func (ch *NoopTestChannel) Underlay() channel.Underlay {
	//TODO implement me
	panic("implement me")
}

func (ch *NoopTestChannel) StartRx() {
}

func (ch *NoopTestChannel) Id() *identity.TokenId {
	panic("implement Id()")
}

func (ch *NoopTestChannel) LogicalName() string {
	panic("implement LogicalName()")
}

func (ch *NoopTestChannel) ConnectionId() string {
	panic("implement ConnectionId()")
}

func (ch *NoopTestChannel) Certificates() []*x509.Certificate {
	panic("implement Certificates()")
}

func (ch *NoopTestChannel) Label() string {
	return "testchannel"
}

func (ch *NoopTestChannel) SetLogicalName(string) {
	panic("implement SetLogicalName")
}

func (ch *NoopTestChannel) Send(channel.Sendable) error {
	return nil
}

func (ch *NoopTestChannel) Close() error {
	panic("implement Close")
}

func (ch *NoopTestChannel) IsClosed() bool {
	panic("implement IsClosed")
}

func (ch *NoopTestChannel) GetTimeSinceLastRead() time.Duration {
	return 0
}
