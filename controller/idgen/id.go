package idgen

import (
	"crypto/rand"
	"encoding/binary"
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
	NextAlphaNumericPrefixedId() (string, error)
}

type shortIdGenerator struct {
	*shortid.Shortid
}

func (self *shortIdGenerator) NextId() (string, error) {
	return self.Generate()
}

func (self *shortIdGenerator) NextAlphaNumericPrefixedId() (string, error) {
	for {
		id, err := self.NextId()
		if err != nil {
			return "", err
		}
		if id[0] != '-' && id[0] != '.' {
			return id, nil
		}
	}
}
