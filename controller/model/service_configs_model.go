package model

import (
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type Config struct {
	Data map[string]interface{}
}

type ServiceConfigs struct {
	BaseModelEntityImpl
	Name    string
	Configs map[string]*Config
}

func (entity *ServiceConfigs) FillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltServiceConfigs, ok := boltEntity.(*persistence.ServiceConfigs)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model service configs", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltServiceConfigs)
	entity.Name = boltServiceConfigs.Name
	return nil
}

func (entity *ServiceConfigs) ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	panic("implement me")
}

func (entity *ServiceConfigs) ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	panic("implement me")
}

func (entity *ServiceConfigs) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	panic("implement me")
}
