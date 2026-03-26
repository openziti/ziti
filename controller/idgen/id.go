package idgen

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/dineshappavoo/basex"
	"github.com/google/uuid"
	"github.com/teris-io/shortid"
)

const Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-"

var idGenerator Generator

func NewGenerator() Generator {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	seed := binary.LittleEndian.Uint64(buf)
	return &shortIdGenerator{
		Shortid: shortid.MustNew(0, Alphabet, seed),
	}
}

func init() {
	idGenerator = NewGenerator()
}

func New() string {
	id, _ := idGenerator.NextId()
	return id
}

type Generator interface {
	NextId() (string, error)
}

type shortIdGenerator struct {
	*shortid.Shortid
}

func (self *shortIdGenerator) NextId() (string, error) {
	for {
		id, err := self.Generate()
		if err != nil {
			return "", err
		}
		if id[0] != '-' && id[0] != '.' {
			return id, nil
		}
	}
}

func MustNewUUIDString() string {
	id := uuid.New()
	v := &big.Int{}
	v.SetBytes(id[:])
	result, err := basex.EncodeInt(v)
	if err != nil {
		panic(err)
	}
	return result
}

func NewUUIDString() (string, error) {
	id := uuid.New()
	v := &big.Int{}
	v.SetBytes(id[:])
	return basex.EncodeInt(v)
}

// NewEpochBytes returns a raw 16-byte UUIDv7. UUIDv7 encodes a millisecond
// timestamp in the high bits, so raw bytes sort chronologically via
// bytes.Compare.
func NewEpochBytes() []byte {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Errorf("failed to generate epoch UUID: %w", err))
	}
	return id[:]
}

// FormatEpoch formats a raw epoch (16-byte UUIDv7) as a UUID string for
// logging and display.
func FormatEpoch(epoch []byte) string {
	if len(epoch) == 16 {
		id, err := uuid.FromBytes(epoch)
		if err == nil {
			return id.String()
		}
	}
	return fmt.Sprintf("%x", epoch)
}
