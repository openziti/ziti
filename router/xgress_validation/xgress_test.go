package xgress_validation

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/forwarder"
	"github.com/openziti/ziti/router/handler_xgress"
	"github.com/openziti/ziti/router/xgress_common"
	"github.com/openziti/ziti/router/xgress_router"
	"github.com/stretchr/testify/require"
	"math/rand/v2"
	"net"
	"testing"
	"time"
)

func newTestSetup() *testSetup {
	result := &testSetup{}
	result.init()
	return result
}

type testSetup struct {
	closeNotify      chan struct{}
	fwd              *forwarder.Forwarder
	dataPlaneAdapter xgress.DataPlaneAdapter
	options          *xgress.Options
}

func (self *testSetup) init() {
	self.closeNotify = make(chan struct{})

	registryConfig := metrics.DefaultUsageRegistryConfig("test", self.closeNotify)
	metricsRegistry := metrics.NewUsageRegistry(registryConfig)

	fwdOptions := env.DefaultForwarderOptions()
	self.fwd = forwarder.NewForwarder(metricsRegistry, nil, fwdOptions, self.closeNotify)
	acker := xgress_router.NewAcker(self.fwd, metricsRegistry, self.closeNotify)
	retransmitter := xgress.NewRetransmitter(self.fwd, metricsRegistry, self.closeNotify)
	payloadIngester := xgress.NewPayloadIngester(self.closeNotify)

	self.dataPlaneAdapter = handler_xgress.NewXgressDataPlaneAdapter(handler_xgress.DataPlaneAdapterConfig{
		Acker:           acker,
		Forwarder:       self.fwd,
		Retransmitter:   retransmitter,
		PayloadIngester: payloadIngester,
		Metrics:         xgress.NewMetrics(metricsRegistry),
	})
	self.options = xgress.DefaultOptions()
}

func (self *testSetup) close() {
	close(self.closeNotify)
}

func (self *testSetup) cleanupCircuit(circuit *testEnv) {
	self.fwd.UnregisterDestinations(circuit.circuitId)

	_ = circuit.clientXgConn.Close()
	_ = circuit.hostXgConn.Close()

	circuit.clientXg.Close()
	circuit.hostXg.Close()
}

func (self *testSetup) createCircuit(t *testing.T, clientF, hostF func(originator xgress.Originator) (net.Conn, xgress.Connection)) *testEnv {
	result := &testEnv{}
	result.closeUp = true
	result.closeDown = true

	result.clientConn, result.clientXgConn = clientF(xgress.Initiator)
	result.circuitId = eid.New()
	result.clientAddr = xgress.Address(eid.New())

	result.clientXg = xgress.NewXgress(result.circuitId, "ctrl1", result.clientAddr, result.clientXgConn, xgress.Initiator, self.options, nil)
	result.clientXg.SetDataPlaneAdapter(self.dataPlaneAdapter)
	self.fwd.RegisterDestination(result.circuitId, result.clientAddr, result.clientXg)

	result.hostConn, result.hostXgConn = hostF(xgress.Terminator)
	result.hostAddr = xgress.Address(eid.New())

	result.hostXg = xgress.NewXgress(result.circuitId, "ctrl1", result.hostAddr, result.hostXgConn, xgress.Terminator, self.options, nil)
	result.hostXg.SetDataPlaneAdapter(self.dataPlaneAdapter)

	self.fwd.RegisterDestination(result.circuitId, result.hostAddr, result.hostXg)

	err := self.fwd.Route("ctrl1", &ctrl_pb.Route{
		CircuitId: result.circuitId,
		Forwards: []*ctrl_pb.Route_Forward{
			{SrcAddress: string(result.clientAddr), DstAddress: string(result.hostAddr)},
			{SrcAddress: string(result.hostAddr), DstAddress: string(result.clientAddr)},
		},
	})

	if err != nil {
		t.Fatal(err)
	}

	result.hostXg.Start()

	result.clientXg.Start()
	return result
}

func (self *testSetup) testVariations(t *testing.T, clientF, hostF func(originator xgress.Originator) (net.Conn, xgress.Connection)) {
	t.Run("data upstream", func(t *testing.T) {
		circuit := self.createCircuit(t, clientF, hostF)
		circuit.sendUpstream = true
		self.runTestVariation(circuit, t)
	})

	t.Run("data downstream", func(t *testing.T) {
		circuit := self.createCircuit(t, clientF, hostF)
		circuit.sendDownstream = true
		self.runTestVariation(circuit, t)
	})

	t.Run("data bidirectional", func(t *testing.T) {
		circuit := self.createCircuit(t, clientF, hostF)
		circuit.sendUpstream = true
		circuit.sendDownstream = true
		self.runTestVariation(circuit, t)
		self.cleanupCircuit(circuit)
	})
}

func (self *testSetup) runTestVariation(env *testEnv, t *testing.T) {
	errs := make(chan error, 5)
	sendUpDoneC := make(chan struct{})
	sendDownDoneC := make(chan struct{})

	reportErr := func(err error) {
		select {
		case errs <- err:
		default:
		}
	}

	if env.sendUpstream {
		blockCh := make(chan []byte, 1000)
		go self.sendBlocks(blockCh, env.clientConn, reportErr)
		go self.recvBlocks(blockCh, env.hostConn, reportErr, sendUpDoneC)
	} else {
		close(sendUpDoneC)
	}

	if env.sendDownstream {
		blockCh := make(chan []byte, 1000)
		go self.sendBlocks(blockCh, env.hostConn, reportErr)
		go self.recvBlocks(blockCh, env.clientConn, reportErr, sendDownDoneC)
	} else {
		close(sendDownDoneC)
	}

	upComplete := false
	downComplete := false

	timeout := time.After(6 * time.Second)

	req := require.New(t)

	for !(upComplete && downComplete) {
		select {
		case err := <-errs:
			req.NoError(err)
		case <-sendUpDoneC:
			upComplete = true
		case <-sendDownDoneC:
			downComplete = true
		case <-timeout:
			t.Fatal("test timed out")
		}
	}

	if env.closeUp {
		req.NoError(env.clientConn.Close())
	}
	if env.closeDown {
		req.NoError(env.hostConn.Close())
	}

	if env.closeUp || env.closeDown {
		start := time.Now()
		for !(env.clientXg.Closed() && env.hostXg.Closed()) {
			time.Sleep(100 * time.Millisecond)
			if time.Since(start) > 10*time.Second {
				debugz.DumpStack()
				req.True(env.clientXg.Closed())
				req.True(env.hostXg.Closed())
				return
			}
		}
	}
}

func (self *testSetup) sendBlocks(blocks chan<- []byte, conn net.Conn, reportErr func(err error)) {
	seed := [32]byte{}
	for i := 0; i < 32; i++ {
		seed[i] = byte(time.Now().UnixMilli() >> (i % 8))
	}
	randSrc := rand.NewChaCha8(seed)
	for i := 0; i < 1000; i++ {
		block := make([]byte, 64+rand.IntN(10000))
		_, _ = randSrc.Read(block)
		blocks <- block
		_, err := conn.Write(block)
		if err != nil {
			reportErr(err)
			return
		}
	}
}

func (self *testSetup) recvBlocks(blocks <-chan []byte, conn net.Conn, reportErr func(err error), doneC chan struct{}) {
	timeout := time.After(time.Second * 5)
	for i := 0; i < 1000; i++ {
		select {
		case block := <-blocks:
			blockCopy := make([]byte, len(block))
			n, err := conn.Read(blockCopy)
			if err != nil {
				reportErr(fmt.Errorf("error reading block (%w)", err))
				return
			}
			if n != len(block) {
				reportErr(fmt.Errorf("read size mismatch, expected %d, got %d", len(block), n))
				return
			}
			if len(block) != len(blockCopy) {
				reportErr(fmt.Errorf("block sizes mismatch, expected %d, got %d", len(block), len(blockCopy)))
				return
			}

			if !bytes.Equal(block, blockCopy) {
				fmt.Printf("block %d: %x\n", i, block[:10])
				fmt.Printf("copy %d: %x\n", i, blockCopy[:10])
				reportErr(fmt.Errorf("block contents did not match for index %d", i))
				return
			}
		case <-timeout:
			reportErr(errors.New("timeout getting next block"))
		}
	}
	close(doneC)
}

func (self *testSetup) NewErtConn(originator xgress.Originator) (net.Conn, xgress.Connection) {
	pipe := NewBufferPipe(originator.String())
	xgConn := xgress_common.NewXgressConn(pipe.Right(), true, false)
	return pipe.Left(), xgConn
}

type testEnv struct {
	circuitId    string
	clientAddr   xgress.Address
	hostAddr     xgress.Address
	clientConn   net.Conn
	hostConn     net.Conn
	clientXgConn xgress.Connection
	hostXgConn   xgress.Connection
	clientXg     *xgress.Xgress
	hostXg       *xgress.Xgress

	sendUpstream   bool
	sendDownstream bool
	closeUp        bool
	closeDown      bool
}

func TestDataFlowAndCloseLogic(t *testing.T) {
	setup := newTestSetup()
	setup.init()
	defer setup.close()

	setup.testVariations(t, setup.NewErtConn, setup.NewErtConn)
}
