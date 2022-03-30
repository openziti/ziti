package boltz

import (
	"github.com/openziti/storage/ast"
	"go.etcd.io/bbolt"
)

type Manager struct {
	Employee
	IsTechLead bool
}

func (entity *Manager) LoadValues(store CrudStore, bucket *TypedBucket) {
	_, err := store.GetParentStore().BaseLoadOneById(bucket.Tx(), entity.Id, &entity.Employee)
	bucket.SetError(err)
	entity.IsTechLead = bucket.GetBoolWithDefault("isTechLead", false)
}

func (entity *Manager) SetValues(ctx *PersistContext) {
	entity.Employee.SetValues(ctx.GetParentContext())
	ctx.SetBool("isTechLead", entity.IsTechLead)
}

type ManagerStore interface {
	LoadOneById(tx *bbolt.Tx, id string) (*Manager, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*Manager, error)
}

func newManagerStore(parent *employeeStoreImpl) *managerStoreImpl {
	entityNotFoundF := func(id string) error {
		return NewNotFoundError(parent.GetSingularEntityType(), "id", id)
	}

	parentMapper := func(entity Entity) Entity {
		if mgr, ok := entity.(*Manager); ok {
			return &mgr.Employee
		}
		return entity
	}

	store := &managerStoreImpl{
		BaseStore: NewChildBaseStore(parent, parentMapper, entityNotFoundF, "ext"),
	}
	store.InitImpl(store)
	return store
}

type managerStoreImpl struct {
	*BaseStore
}

func (store *managerStoreImpl) NewStoreEntity() Entity {
	return &Manager{}
}

func (store *managerStoreImpl) initializeLocal() {
	store.GetParentStore().GrantSymbols(store)
	store.AddSymbol("isTechLead", ast.NodeTypeBool)
}

func (store *managerStoreImpl) initializeLinked() {
	// does nothing
}

func (store *managerStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Manager, error) {
	entity := &Manager{}
	found, err := store.BaseLoadOneById(tx, id, entity)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, NewNotFoundError(store.GetEntityType(), "id", id)
	}
	return entity, nil
}
