package persistence

import (
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldConfigData = "data"
)

type ServiceConfigs struct {
	BaseEdgeEntityImpl
	Name string
}

func (entity *ServiceConfigs) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
}

func (entity *ServiceConfigs) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
}

func (entity *ServiceConfigs) GetEntityType() string {
	return EntityTypeServiceConfigs
}

type Config struct {
	BaseEdgeEntityImpl
	Data map[string]interface{}
}

func (entity *Config) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.Data = bucket.GetMap(FieldConfigData)
}

func (entity *Config) SetValues(ctx *boltz.PersistContext) {
	ctx.SetMap(FieldConfigData, entity.Data)
}

func (entity *Config) GetEntityType() string {
	return EntityTypeConfigs
}

type ServiceConfigsStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*ServiceConfigs, error)
	LoadOneByName(tx *bbolt.Tx, name string) (*ServiceConfigs, error)
	GetNameIndex() boltz.ReadIndex
}

func newServiceConfigsStore(stores *stores) *serviceConfigsStoreImpl {
	store := &serviceConfigsStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeConfigs),
	}
	store.InitImpl(store)
	return store
}

type serviceConfigsStoreImpl struct {
	*baseStore

	indexName boltz.ReadIndex
}

func (store *serviceConfigsStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *serviceConfigsStoreImpl) initializeLocal() {
	store.addBaseFields()
	store.indexName = store.addUniqueNameField()
}

func (store *serviceConfigsStoreImpl) initializeLinked() {
}

func (store *serviceConfigsStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &ServiceConfigs{}
}

func (store *serviceConfigsStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*ServiceConfigs, error) {
	entity := &ServiceConfigs{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *serviceConfigsStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*ServiceConfigs, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *serviceConfigsStoreImpl) GetConfigs(tx *bbolt.Tx, serviceConfigsId string, configIds ...string) (map[string]*Config, error) {
	result := map[string]*Config{}
	for _, configId := range configIds {
		config := &Config{}
		found, err := store.BaseLoadOneChildById(tx, serviceConfigsId, configId, config)
		if err != nil {
			return nil, err
		}
		if found {
			result[configId] = config
		}
	}
	return result, nil
}
