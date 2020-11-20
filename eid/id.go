package eid

import (
	"github.com/teris-io/shortid"
	"math/rand"
)

const Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-"

var idGenerator *shortid.Shortid

func init() {
	idGenerator = shortid.MustNew(0, Alphabet, rand.Uint64())
}

func New() string {
	id, _ := idGenerator.Generate()
	return id
}
