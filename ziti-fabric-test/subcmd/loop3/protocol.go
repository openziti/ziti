/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-cmd/ziti-fabric-test/subcmd/loop3/pb"
	"github.com/netfoundry/ziti-foundation/util/info"
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/pkg/errors"
	"github.com/rcrowley/go-metrics"
	"io"
	"math/rand"
	"time"
)

type Block struct {
	Sequence int32  `protobuf:"varint,1,opt,name=sequence,proto3" json:"sequence,omitempty"`
	Data     []byte `protobuf:"bytes,2,opt,name=data,proto3" json:"data,omitempty"`
	Hash     []byte `protobuf:"bytes,3,opt,name=hash,proto3" json:"hash,omitempty"`
}

type protocol struct {
	test        *loop3_pb.Test
	txGenerator *generator
	rxSequence  int32
	peer        io.ReadWriteCloser
	rxBlocks    chan *Block
	txCount     int32
	rxCount     int32
	errors      chan error
}

var rxRate = metrics.NewMeter()
var txRate = metrics.NewMeter()

func init() {
	log := pfxlog.Logger()
	if err := metrics.Register("rxRate", rxRate); err != nil {
		log.WithError(err).Error("failed to register rx rate metrics")
	}
	if err := metrics.Register("txRate", txRate); err != nil {
		log.WithError(err).Error("failed to register tx rate metrics")
	}

	go func() {
		mbs := float64(1024 * 1024)
		for range time.Tick(time.Second * 5) {
			metrics.Each(func(name string, i interface{}) {
				switch metric := i.(type) {
				case metrics.Meter:
					if metric.Rate1() > mbs {
						log.Infof("%v - count: %v, 1m: %.2f MB/s, 5m: %.2f MB/s, avg: %.2f, MB/s",
							name, metric.Count(), metric.Rate1()/mbs, metric.Rate5()/mbs, metric.RateMean()/mbs)
					} else {
						log.Infof("%v - count: %v, 1m: %.2f KB/s, 5m: %.2f KB/s, avg: %.2f /KB/s",
							name, metric.Count(), metric.Rate1()/1024, metric.Rate5()/1024, metric.RateMean()/1024)
					}
				}
			})
		}
	}()
}

var MagicHeader = []byte{0xCA, 0xFE, 0xF0, 0x0D}

func newProtocol(peer io.ReadWriteCloser) (*protocol, error) {
	p := &protocol{
		rxSequence: 0,
		peer:       peer,
		rxBlocks:   make(chan *Block, 128),
		txCount:    0,
		rxCount:    0,
		errors:     make(chan error, 10240),
	}
	return p, nil
}

func (p *protocol) run(test *loop3_pb.Test) error {
	p.test = test
	p.txGenerator = newGenerator(int(test.TxRequests), int(test.PayloadMinBytes), int(test.PayloadMaxBytes))
	go p.txGenerator.run()

	rxerDone := make(chan bool)
	go p.rxer(rxerDone)
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

	for p.txCount < p.test.TxRequests {
		select {
		case block := <-p.txGenerator.blocks:
			if block != nil {
				if err := p.txBlock(block); err == nil {
					p.txCount++

					if p.test.TxPacing > 0 {
						jitter := 0
						if p.test.TxMaxJitter > 0 {
							jitter = rand.Intn(int(p.test.TxMaxJitter))
						}
						time.Sleep(time.Duration(int(p.test.TxPacing)+jitter) * time.Millisecond)
					}

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
	}

	log.Info("tx count reached")
}

func (p *protocol) rxer(done chan bool) {
	log := pfxlog.ContextLogger(p.test.Name)
	log.Debug("started")
	defer func() { done <- true }()
	defer log.Debug("complete")

	for p.rxCount < p.test.RxRequests {
		block, err := p.rxBlock()
		if err != nil {
			p.errors <- err
			log.Error(err)
			return
		}
		p.rxBlocks <- block
		p.rxCount++
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
				if block.Sequence == p.rxSequence {
					hash := sha512.Sum512(block.Data)
					if hex.EncodeToString(hash[:]) != hex.EncodeToString(block.Hash) {
						err := errors.New("mismatched hashes")
						p.errors <- err
						if closeErr := p.peer.Close(); closeErr != nil {
							log.Error(closeErr)
						}
						log.Error(err)
						return
					}
					p.rxSequence++

				} else {
					err := fmt.Errorf("expected sequence [%d] got sequence [%d]", p.rxSequence, block.Sequence)
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
			err := fmt.Errorf("rx timeout exceeded (%d ms.)", p.test.RxTimeout)
			p.errors <- err
			if closeErr := p.peer.Close(); closeErr != nil {
				log.Error(closeErr)
			}
			log.Error(err)
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

func (p *protocol) rxBlock() (*Block, error) {
	if err := p.rxMagicHeader(); err != nil {
		return nil, err
	}
	seq, err := p.rxInt32()
	if err != nil {
		return nil, err
	}

	data, err := p.rxByteArray()
	if err != nil {
		return nil, err
	}

	hash, err := p.rxByteArray()
	if err != nil {
		return nil, err
	}

	block := &Block{
		Sequence: seq,
		Data:     data,
		Hash:     hash,
	}
	pfxlog.ContextLogger(p.test.Name).Debugf("<- #%d (%s)", block.Sequence, info.ByteCount(int64(len(block.Data))))

	// header (4) + seq (4) + data (4 + data len) + hash (4 + hash len)
	rxRate.Mark(int64(16 + len(block.Data) + len(block.Hash)))

	return block, nil
}

func (p *protocol) txResult(result *loop3_pb.Result) error {
	if err := p.txPb(result); err != nil {
		return err
	}
	if result.Success {
		pfxlog.ContextLogger(p.test.Name).Infof("<- [result+]")
	} else {
		pfxlog.ContextLogger(p.test.Name).Infof("<- [result-]")
	}
	return nil
}

func (p *protocol) rxResult() (*loop3_pb.Result, error) {
	result := &loop3_pb.Result{}
	if err := p.rxPb(result); err != nil {
		return nil, err
	}
	if result.Success {
		pfxlog.ContextLogger(p.test.Name).Infof("<- [result+]")
	} else {
		pfxlog.ContextLogger(p.test.Name).Infof("<- [result-]")
	}
	return result, nil
}

func (p *protocol) txBlock(block *Block) error {
	if err := p.txMagicHeader(); err != nil {
		return err
	}
	if err := p.txInt32(block.Sequence); err != nil {
		return err
	}
	if err := p.txByteArray(block.Data); err != nil {
		return err
	}
	if err := p.txByteArray(block.Hash); err != nil {
		return err
	}

	// header (4) + seq (4) + data (4 + data len) + hash (4 + hash len)
	txRate.Mark(int64(16 + len(block.Data) + len(block.Hash)))

	pfxlog.ContextLogger(p.test.Name).Debugf("-> #%d (%s)", block.Sequence, info.ByteCount(int64(len(block.Data))))
	return nil
}

func (p *protocol) txPb(pb proto.Message) error {
	data, err := proto.Marshal(pb)
	if err != nil {
		return err
	}
	if err = p.txMagicHeader(); err != nil {
		return err
	}
	return p.txByteArray(data)
}

func (p *protocol) rxPb(pb proto.Message) error {
	if err := p.rxMagicHeader(); err != nil {
		return err
	}
	data, err := p.rxByteArray()
	if err != nil {
		return err
	}
	if err := proto.Unmarshal(data, pb); err != nil {
		return err
	}
	return nil
}

func (p *protocol) rxByteArray() ([]byte, error) {
	length, err := p.rxLength()
	if err != nil {
		return nil, err
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
		return nil, err
	}
	if n != length {
		return nil, fmt.Errorf("short data read [%d != %d]", n, length)
	}
	return data, nil
}

func (p *protocol) txMagicHeader() error {
	n, err := p.peer.Write(MagicHeader)
	if err != nil {
		return err
	}
	if n != len(MagicHeader) {
		return errors.New("short data write (magic header)")
	}
	return nil
}

func (p *protocol) txByteArray(bytes []byte) error {
	dataLen := len(bytes)
	if err := p.txLength(dataLen); err != nil {
		return err
	}

	n, err := p.peer.Write(bytes)
	if err != nil {
		return err
	}
	if n != dataLen {
		return errors.New("short length write")
	}
	return nil
}

func (p *protocol) txLength(len int) error {
	return p.txInt32(int32(len))
}

func (p *protocol) txInt32(len int32) error {
	out := new(bytes.Buffer)
	if err := binary.Write(out, binary.LittleEndian, len); err != nil {
		return err
	}
	n, err := p.peer.Write(out.Bytes())
	if err != nil {
		return err
	}
	if n != out.Len() {
		return errors.New("short length write")
	}
	return nil
}

func (p *protocol) rxLength() (int, error) {
	result, err := p.rxInt32()
	return int(result), err
}

func (p *protocol) rxInt32() (int32, error) {
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
	return length, nil
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
