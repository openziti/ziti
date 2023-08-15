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

package loop2

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/ziti/zititest/ziti-fabric-test/subcmd/loop2/pb"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"io"
	"math/rand"
	"time"
)

type protocol struct {
	test        *loop2_pb.Test
	txGenerator *generator
	rxSequence  int32
	peer        io.ReadWriteCloser
	rxBlocks    chan *loop2_pb.Block
	txCount     int32
	rxCount     int32
	errors      chan error
}

var MagicHeader = []byte{0xCA, 0xFE, 0xF0, 0x0D}

func newProtocol(peer io.ReadWriteCloser) (*protocol, error) {
	p := &protocol{
		rxSequence: 0,
		peer:       peer,
		rxBlocks:   make(chan *loop2_pb.Block),
		txCount:    0,
		rxCount:    0,
		errors:     make(chan error, 10240),
	}
	return p, nil
}

func (p *protocol) run(test *loop2_pb.Test) error {
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
		block := <-p.txGenerator.blocks
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

func (p *protocol) txTest(test *loop2_pb.Test) error {
	if err := p.txPb(test); err != nil {
		return err
	}
	pfxlog.Logger().Info("-> [test]")
	return nil
}

func (p *protocol) rxTest() (*loop2_pb.Test, error) {
	test := &loop2_pb.Test{}
	if err := p.rxPb(test); err != nil {
		return nil, err
	}
	pfxlog.Logger().Infof("<- [test]")
	return test, nil
}

func (p *protocol) txBlock(block *loop2_pb.Block) error {
	if err := p.txPb(block); err != nil {
		return err
	}
	pfxlog.ContextLogger(p.test.Name).Infof("-> #%d (%s)", block.Sequence, info.ByteCount(int64(len(block.Data))))
	return nil
}

func (p *protocol) rxBlock() (*loop2_pb.Block, error) {
	block := &loop2_pb.Block{}
	if err := p.rxPb(block); err != nil {
		return nil, err
	}
	pfxlog.ContextLogger(p.test.Name).Infof("<- #%d (%s)", block.Sequence, info.ByteCount(int64(len(block.Data))))
	return block, nil
}

func (p *protocol) txResult(result *loop2_pb.Result) error {
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

func (p *protocol) rxResult() (*loop2_pb.Result, error) {
	result := &loop2_pb.Result{}
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

func (p *protocol) txPb(pb proto.Message) error {
	data, err := proto.Marshal(pb)

	if err != nil {
		return err
	}
	if err = p.txMagicHeader(); err != nil {
		return err
	}
	if err := p.txLength(len(data)); err != nil {
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

func (p *protocol) txLength(len int) error {
	out := new(bytes.Buffer)
	if err := binary.Write(out, binary.LittleEndian, int32(len)); err != nil {
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
