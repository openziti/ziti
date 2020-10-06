package loop3

import (
	"bytes"
	"encoding/binary"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/info"
	"github.com/pkg/errors"
	"time"
)

type Message interface {
	Tx(p *protocol) error
	Rx(p *protocol) error
}

var success = []byte{1}
var failure = []byte{0}

type Result struct {
	Success bool
	Message string
}

func (r *Result) getSuccessBytes() []byte {
	if r.Success {
		return success
	}
	return failure
}

func (r *Result) Tx(p *protocol) error {
	dataLen := 1 + len(r.Message)
	if err := p.txHeader(dataLen); err != nil {
		return err
	}

	if _, err := p.peer.Write(r.getSuccessBytes()); err != nil {
		return err
	}

	if len(r.Message) > 0 {
		if _, err := p.peer.Write([]byte(r.Message)); err != nil {
			return err
		}
	}

	MsgTxRate.Mark(1)
	BytesTxRate.Mark(int64(4 + 4 + dataLen))

	if r.Success {
		pfxlog.ContextLogger(p.test.Name).Infof("<- [result+]")
	} else {
		pfxlog.ContextLogger(p.test.Name).Infof("<- [result-]")
	}

	return nil
}

func (r *Result) Rx(p *protocol) error {
	msgLen, err := p.rxHeader()
	if err != nil {
		return err
	}
	if msgLen < 1 {
		return errors.Errorf("not enough data to deserialize Result need at least one byte")
	}
	body, err := p.rxMsgBody(msgLen)
	if err != nil {
		return err
	}
	r.Success = body[0] == 1
	r.Message = string(body[1:])

	MsgRxRate.Mark(1)
	BytesRxRate.Mark(int64(4 + 4 + msgLen))

	if r.Success {
		pfxlog.ContextLogger(p.test.Name).Infof("<- [result+]")
	} else {
		pfxlog.ContextLogger(p.test.Name).Infof("<- [result-]")
	}

	return nil
}

const (
	BlockTypePlain           byte = 1
	BlockTypeLatencyRequest       = 2
	BlockTypeLatencyResponse      = 3
)

type Block struct {
	Type      byte
	Sequence  uint32
	Hash      []byte
	Data      []byte
	Timestamp time.Time
}

func (block *Block) getTimestampBytes() ([]byte, error) {
	if block.Type == BlockTypeLatencyRequest {
		block.Timestamp = time.Now()
	}

	tsBuf := bytes.Buffer{}

	if block.Type != BlockTypePlain {
		ts, err := block.Timestamp.MarshalBinary()
		if err != nil {
			return nil, err
		}

		if len(ts) > 32 {
			panic(errors.Errorf("unexpected timestamp length: %v", len(ts)))
		}

		tsBuf.WriteByte(byte(len(ts)))
		tsBuf.Write(ts)
	}
	return tsBuf.Bytes(), nil
}

func (block *Block) Tx(p *protocol) error {
	tsBytes, err := block.getTimestampBytes()
	if err != nil {
		return err
	}

	dataLen := 1 /* block type */ + len(tsBytes) + 4 /* sequence bytes */ + len(block.Hash) + len(block.Data)

	if err := p.txHeader(dataLen); err != nil {
		return err
	}

	if _, err := p.peer.Write([]byte{block.Type}); err != nil {
		return err
	}

	if len(tsBytes) > 0 {
		if _, err := p.peer.Write(tsBytes); err != nil {
			return err
		}
	}

	seqBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(seqBytes, block.Sequence)

	if _, err := p.peer.Write(seqBytes); err != nil {
		return err
	}

	if _, err := p.peer.Write(block.Hash); err != nil {
		return err
	}

	if _, err := p.peer.Write(block.Data); err != nil {
		return err
	}

	MsgTxRate.Mark(1)
	BytesTxRate.Mark(int64(8 + dataLen))

	pfxlog.ContextLogger(p.test.Name).Infof("-> #%d (%s)", block.Sequence, info.ByteCount(int64(len(block.Data))))

	return nil
}

func (block *Block) Rx(p *protocol) error {
	length, err := p.rxHeader()
	if err != nil {
		return err
	}

	body, err := p.rxMsgBody(length)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(body)
	block.Type, err = buf.ReadByte()
	if err != nil {
		return err
	}

	if block.Type != BlockTypePlain {
		tsLen, err := buf.ReadByte()
		if err != nil {
			return err
		}
		if err := block.Timestamp.UnmarshalBinary(buf.Next(int(tsLen))); err != nil {
			return err
		}
	}

	seqBytes := buf.Next(4)
	block.Sequence = binary.LittleEndian.Uint32(seqBytes)

	block.Hash = buf.Next(64)
	block.Data = buf.Bytes()

	MsgRxRate.Mark(1)
	BytesRxRate.Mark(int64(8 + length))

	if block.Type == BlockTypeLatencyResponse {
		elapsed := time.Now().Sub(block.Timestamp)
		MsgLatency.Update(elapsed)
	}

	pfxlog.ContextLogger(p.test.Name).Infof("<- #%d (%s)", block.Sequence, info.ByteCount(int64(len(block.Data))))

	return nil
}
