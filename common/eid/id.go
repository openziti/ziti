package eid

import (
	"crypto/rand"
	"encoding/binary"
	"github.com/teris-io/shortid"
)

const Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-"

var idGenerator *shortid.Shortid

func init() {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	seed := binary.LittleEndian.Uint64(buf)
	idGenerator = shortid.MustNew(0, Alphabet, seed)
}

func New() string {
	id, _ := idGenerator.Generate()
	return id
}
