package model

import (
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/xt"
	"go.etcd.io/bbolt"
)

type Terminator struct {
	models.BaseEntity
	Service         string
	Router          string
	Binding         string
	Address         string
	InstanceId      string
	InstanceSecret  []byte
	Cost            uint16
	Precedence      xt.Precedence
	PeerData        map[uint32][]byte
	HostId          string
	SavedPrecedence xt.Precedence
}

func (entity *Terminator) GetServiceId() string {
	return entity.Service
}

func (entity *Terminator) GetRouterId() string {
	return entity.Router
}

func (entity *Terminator) GetBinding() string {
	return entity.Binding
}

func (entity *Terminator) GetAddress() string {
	return entity.Address
}

func (entity *Terminator) GetInstanceId() string {
	return entity.InstanceId
}

func (entity *Terminator) GetInstanceSecret() []byte {
	return entity.InstanceSecret
}

func (entity *Terminator) GetCost() uint16 {
	return entity.Cost
}

func (entity *Terminator) GetPrecedence() xt.Precedence {
	return entity.Precedence
}

func (entity *Terminator) GetPeerData() xt.PeerData {
	return entity.PeerData
}

func (entity *Terminator) GetHostId() string {
	return entity.HostId
}

func (entity *Terminator) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.Terminator, error) {
	return entity.toBoltEntityForCreate(tx, env)
}

func (entity *Terminator) toBoltEntityForCreate(*bbolt.Tx, Env) (*db.Terminator, error) {
	precedence := xt.Precedences.Default.String()
	if entity.Precedence != nil {
		precedence = entity.Precedence.String()
	}

	var savedPrecedence *string
	if entity.SavedPrecedence != nil {
		precedenceStr := entity.SavedPrecedence.String()
		savedPrecedence = &precedenceStr
	}

	return &db.Terminator{
		BaseExtEntity:   *entity.ToBoltBaseExtEntity(),
		Service:         entity.Service,
		Router:          entity.Router,
		Binding:         entity.Binding,
		Address:         entity.Address,
		InstanceId:      entity.InstanceId,
		InstanceSecret:  entity.InstanceSecret,
		Cost:            entity.Cost,
		Precedence:      precedence,
		PeerData:        entity.PeerData,
		HostId:          entity.HostId,
		SavedPrecedence: savedPrecedence,
	}, nil
}

func (entity *Terminator) fillFrom(_ Env, _ *bbolt.Tx, boltTerminator *db.Terminator) error {
	entity.Service = boltTerminator.Service
	entity.Router = boltTerminator.Router
	entity.Binding = boltTerminator.Binding
	entity.Address = boltTerminator.Address
	entity.InstanceId = boltTerminator.InstanceId
	entity.InstanceSecret = boltTerminator.InstanceSecret
	entity.PeerData = boltTerminator.PeerData
	entity.Cost = boltTerminator.Cost
	entity.Precedence = xt.GetPrecedenceForName(boltTerminator.Precedence)
	entity.HostId = boltTerminator.HostId
	entity.FillCommon(boltTerminator)

	if boltTerminator.SavedPrecedence != nil {
		entity.SavedPrecedence = xt.GetPrecedenceForName(*boltTerminator.SavedPrecedence)
	}

	return nil
}
