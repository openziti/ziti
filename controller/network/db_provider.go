package network

import (
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
)

type DbProvider interface {
	GetDb() boltz.Db
	GetStores() *db.Stores
	GetManagers() *Managers
}
