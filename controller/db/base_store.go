package db

import (
	"github.com/netfoundry/ziti-foundation/storage/boltz"
)

type store interface {
	boltz.CrudStore
}

type baseStore struct {
	stores *stores
	*boltz.BaseStore
}
