package boltz

import (
	"github.com/openziti/storage/ast"
	"github.com/pkg/errors"
)

type Location struct {
	Id string
}

func (entity *Location) GetId() string {
	return entity.Id
}

func (entity *Location) SetId(id string) {
	entity.Id = id
}

func (entity *Location) LoadValues(CrudStore, *TypedBucket) {
}

func (entity *Location) SetValues(*PersistContext) {
}

func (entity *Location) GetEntityType() string {
	return entityTypeLocation
}

func newLocationStore() *locationStoreImpl {
	store := &locationStoreImpl{
		BaseStore: NewBaseStore(entityTypeLocation, func(id string) error {
			return errors.Errorf("entity of type %v with id %v not found", entityTypeLocation, id)
		}, "stores"),
	}
	store.InitImpl(store)
	return store
}

type locationStoreImpl struct {
	*BaseStore
	stores              *testStores
	symbolEmployees     EntitySetSymbol
	employeesCollection LinkCollection
}

func (store *locationStoreImpl) NewStoreEntity() Entity {
	return &Location{}
}

func (store *locationStoreImpl) initializeLocal() {
	store.AddIdSymbol("id", ast.NodeTypeString)
	store.symbolEmployees = store.AddFkSetSymbol(entityTypeEmployee, store.stores.employee)
}

func (store *locationStoreImpl) initializeLinked() {
	store.employeesCollection = store.AddLinkCollection(store.symbolEmployees, store.stores.employee.symbolLocations)
}
