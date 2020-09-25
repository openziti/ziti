package xtv

import "go.etcd.io/bbolt"

type DefaultValidator struct{}

func (d DefaultValidator) Validate(*bbolt.Tx, Terminator) error {
	return nil
}
