package xtv

import (
	"go.etcd.io/bbolt"
)

type DefaultValidator struct{}

func (d DefaultValidator) Validate(*bbolt.Tx, Terminator, bool) error {
	return nil
}
