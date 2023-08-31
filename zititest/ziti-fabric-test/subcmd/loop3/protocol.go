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

package loop3

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/ziti/zititest/ziti-fabric-test/subcmd/loop3/pb"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"io"
	"math/rand"
	"sync/atomic"
	"time"
)

type protocol struct {
	test         *loop3_pb.Test
	blocks       chan Block
	rxSequence   uint64
	txPacing     time.Duration
	txMaxJitter  time.Duration
	txPauseEvery time.Duration
	txPauseFor   time.Duration
	rxPacing     time.Duration
	rxMaxJitter  time.Duration
	rxPauseEvery time.Duration
	rxPauseFor   time.Duration
	peer         io.ReadWriteCloser
	rxBlocks     chan Block
	txCount      int32
	rxCount      int32
	lastRx       int64
	latencies    chan *time.Time
	errors       chan error
}

var MagicHeader = []byte{0xCA, 0xFE, 0xF0, 0x0D}

func newProtocol(peer io.ReadWriteCloser) (*protocol, error) {
	p := &protocol{
		rxSequence: 0,
		peer:       peer,
		rxBlocks:   make(chan Block),
		txCount:    0,
		rxCount:    0,
		latencies:  make(chan *time.Time, 1024),
		errors:     make(chan error, 10240),
	}
	return p, nil
}

func (p *protocol) run(test *loop3_pb.Test) error {
	p.test = test

	var rxBlock func() (Block, error)

	if test.IsTxRandomHashed() {
		txGenerator := newRandomHashedBlockGenerator(int(test.TxRequests), int(test.PayloadMinBytes), int(test.PayloadMaxBytes), int(test.LatencyFrequency))
		p.blocks = txGenerator.blocks
		go txGenerator.run()
	} else if test.IsTxSequential() {
		txGenerator := newSeqGenerator(int(test.TxRequests), int(test.PayloadMinBytes), int(test.PayloadMaxBytes))
		p.blocks = txGenerator.blocks
		go txGenerator.run()
	} else {
		panic(errors.Errorf("unknown tx block type %v", test.TxBlockType))
	}

	if test.IsRxRandomHashed() {
		rxBlock = p.rxRandomHashedBlock
	} else if test.IsRxSequential() {
		rxBlock = p.rxSeqBlock
	} else {
		panic(errors.Errorf("unknown rx block type %v", test.RxBlockType))
	}

	parseTime := func(val string) time.Duration {
		t, err := time.ParseDuration(val)
		if err != nil {
			panic(err)
		}
		return t
	}

	p.txPacing = parseTime(p.test.TxPacing)
	p.txMaxJitter = parseTime(p.test.TxMaxJitter)
	p.txPauseEvery = parseTime(p.test.TxPauseEvery)
	p.txPauseFor = parseTime(p.test.TxPauseFor)

	p.rxPacing = parseTime(p.test.RxPacing)
	p.rxMaxJitter = parseTime(p.test.RxMaxJitter)
	p.rxPauseEvery = parseTime(p.test.RxPauseEvery)
	p.rxPauseFor = parseTime(p.test.RxPauseFor)

	rxerDone := make(chan bool)
	go p.rxer(rxerDone, rxBlock)
	if p.test.RxRequests > 0 {
		go p.verifier()
	}

	txerDone := make(chan bool)
	go p.txer(txerDone)

	<-rxerDone
	<-txerDone

	if len(p.errors) > 0 {
		err := <-p.errors
		return err
	}

	return nil
}

func (p *protocol) txer(done chan bool) {
	log := pfxlog.ContextLogger(p.test.Name)
	log.Debug("started")
	defer func() { done <- true }()
	defer log.Debug("complete")

	var lastSend time.Time
	lastPause := time.Now()
	for p.txCount < p.test.TxRequests {
		now := time.Now()
		if p.txPauseEvery > 0 && now.Sub(lastPause) > p.txPauseEvery {
			time.Sleep(p.txPauseFor)
			lastPause = time.Now()
		}
		block := <-p.blocks
		if block != nil {
			if p.txPacing > 0 {
				jitter := time.Duration(0)
				if p.txMaxJitter > 0 {
					jitter = time.Duration(rand.Intn(int(p.txMaxJitter)))
				}

				nextSend := lastSend.Add(p.txPacing + jitter)
				if nextSend.After(now) {
					time.Sleep(nextSend.Sub(now))
					lastSend = nextSend
				} else {
					lastSend = now
				}
			}

			block.PrepForSend(p)
			if err := block.Tx(p); err == nil {
				atomic.AddInt32(&p.txCount, 1)
			} else {
				log.Errorf("error sending block (%s)", err)
				p.errors <- err
				return
			}
		} else {
			log.Errorf("tx blocks chan closed")
			return
		}
	}

	log.Info("tx count reached")
}

func (p *protocol) rxer(done chan bool, rxBlock func() (Block, error)) {
	log := pfxlog.ContextLogger(p.test.Name)
	log.Debug("started")
	defer func() { done <- true }()
	defer log.Debug("complete")

	lastRx := time.Now()
	lastPause := time.Now()
	for p.rxCount < p.test.RxRequests {
		now := time.Now()
		if p.rxPauseEvery > 0 && now.Sub(lastPause) > p.rxPauseEvery {
			time.Sleep(p.rxPauseFor)
			lastPause = time.Now()
		}
		block, err := rxBlock()
		if err != nil {
			p.errors <- err
			log.Error(err)
			return
		}

		atomic.AddInt32(&p.rxCount, 1)
		atomic.StoreInt64(&p.lastRx, info.NowInMilliseconds())
		p.rxBlocks <- block

		if p.rxPacing > 0 {
			jitter := time.Duration(0)
			if p.rxMaxJitter > 0 {
				jitter = time.Duration(rand.Intn(int(p.rxMaxJitter)))
			}

			now := time.Now()
			nextRx := lastRx.Add(p.rxPacing + jitter)
			if nextRx.After(now) {
				time.Sleep(nextRx.Sub(now))
				lastRx = nextRx
			} else {
				lastRx = now
			}
		}
	}

	close(p.rxBlocks)
	log.Info("rx count reached")
}

func (p *protocol) verifier() {
	log := pfxlog.ContextLogger(p.test.Name)
	log.Debug("started")
	defer log.Debug("complete")

	for {
		select {
		case block := <-p.rxBlocks:
			if block != nil {
				if err := block.Verify(p); err != nil {
					p.errors <- err
					if closeErr := p.peer.Close(); closeErr != nil {
						log.Error(closeErr)
					}
					log.Error(err)
					return
				}
			} else {
				return
			}

		case <-time.After(time.Duration(p.test.RxTimeout) * time.Millisecond):
			timeSinceLastRx := info.NowInMilliseconds() - atomic.LoadInt64(&p.lastRx)
			errStr := fmt.Sprintf("rx timeout exceeded (%d ms.). Last rx: %v. tx count: %v, rx count: %v",
				p.test.RxTimeout, timeSinceLastRx, atomic.LoadInt32(&p.txCount), atomic.LoadInt32(&p.rxCount))
			// err := errors.New(errStr)
			log.Errorf(errStr)
			// p.errors <- err
			//if closeErr := p.peer.Close(); closeErr != nil {
			//	log.Error(closeErr)
			//}
			return
		}
	}
}

func (p *protocol) txTest(test *loop3_pb.Test) error {
	if err := p.txPb(test); err != nil {
		return err
	}
	pfxlog.Logger().Info("-> [test]")
	return nil
}

func (p *protocol) rxTest() (*loop3_pb.Test, error) {
	test := &loop3_pb.Test{}
	if err := p.rxPb(test); err != nil {
		return nil, err
	}
	pfxlog.Logger().Infof("<- [test]")
	return test, nil
}

func (p *protocol) rxRandomHashedBlock() (Block, error) {
	block := &RandHashedBlock{}
	if err := block.Rx(p); err != nil {
		return nil, err
	}

	if block.Type == BlockTypeLatencyRequest {
		select {
		case p.latencies <- &block.Timestamp:
		default:
			pfxlog.Logger().Warn("latency channel out of room")
		}
	}

	return block, nil
}

func (p *protocol) rxSeqBlock() (Block, error) {
	block := make([]byte, p.test.RxSeqBlockSize)
	_, err := io.ReadFull(p.peer, block)
	if err != nil {
		return nil, err
	}

	pfxlog.ContextLogger(p.test.Name).Infof("<- #%d (%s)", p.rxSequence, info.ByteCount(int64(len(block))))

	return SeqBlock(block), nil
}

func (p *protocol) rxResult() (*Result, error) {
	result := &Result{}
	if err := result.Rx(p); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *protocol) txPb(pb proto.Message) error {
	data, err := proto.Marshal(pb)

	if err != nil {
		return err
	}
	if err = p.txMagicHeader(p.peer); err != nil {
		return err
	}
	if err := p.txLength(p.peer, len(data)); err != nil {
		return err
	}
	n, err := p.peer.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return errors.New("short data write")
	}
	return nil
}

func (p *protocol) rxPb(pb proto.Message) error {
	if err := p.rxMagicHeader(); err != nil {
		return err
	}
	length, err := p.rxLength()
	if err != nil {
		return err
	}

	defer func() {
		if err := recover(); err != nil {
			pfxlog.ContextLogger(p.test.Name).Errorf("failure while reading message of length %v", length)
			panic(err)
		}
	}()

	data := make([]byte, length)
	n, err := io.ReadFull(p.peer, data)
	if err != nil {
		return err
	}
	if n != length {
		return fmt.Errorf("short data read [%d != %d]", n, length)
	}
	if err := proto.Unmarshal(data, pb); err != nil {
		return err
	}
	return nil
}

func (p *protocol) txMagicHeader(w io.Writer) error {
	n, err := w.Write(MagicHeader)
	if err != nil {
		return err
	}
	if n != len(MagicHeader) {
		return errors.New("short data write (magic header)")
	}
	return nil
}

func (p *protocol) txLength(w io.Writer, length int) error {
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, uint32(length))
	n, err := w.Write(out)
	if err != nil {
		return err
	}
	if n != 4 {
		return errors.New("short length write")
	}
	return nil
}

func (p *protocol) txHeader(w io.Writer, msgLen int) error {
	if err := p.txMagicHeader(w); err != nil {
		return err
	}

	if err := p.txLength(w, msgLen); err != nil {
		return err
	}
	return nil
}

func (p *protocol) rxLength() (int, error) {
	data := make([]byte, 4)
	n, err := io.ReadFull(p.peer, data)
	if err != nil {
		return -1, err
	}
	if n != 4 {
		return -1, fmt.Errorf("short length read [%d != 4]", n)
	}
	buf := bytes.NewBuffer(data)
	var length int32 = -1
	err = binary.Read(buf, binary.LittleEndian, &length)
	if err != nil {
		return -1, err
	}
	return int(length), nil
}

func (p *protocol) rxMagicHeader() error {
	data := make([]byte, len(MagicHeader))
	n, err := io.ReadFull(p.peer, data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return fmt.Errorf("short magic header read [%v != %v]", n, len(data))
	}
	if !bytes.Equal(MagicHeader, data) {
		return errors.Errorf("bad header. Got %v, expected %v", data, MagicHeader)
	}
	return nil
}

func (p *protocol) rxHeader() (int, error) {
	if err := p.rxMagicHeader(); err != nil {
		return 0, err
	}
	return p.rxLength()
}

func (p *protocol) rxMsgBody(length int) ([]byte, error) {
	data := make([]byte, length)
	_, err := io.ReadFull(p.peer, data)
	if err != nil {
		return nil, err
	}
	return data, err
}
