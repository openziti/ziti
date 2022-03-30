package boltz

import (
	"github.com/openziti/storage/ast"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"sort"
)

type Employee struct {
	Id             string
	Name           string
	ManagerId      *string
	RoleAttributes []string
}

func (entity *Employee) GetId() string {
	return entity.Id
}

func (entity *Employee) SetId(id string) {
	entity.Id = id
}

func (entity *Employee) LoadValues(_ CrudStore, bucket *TypedBucket) {
	entity.Name = bucket.GetStringOrError(fieldName)
	entity.ManagerId = bucket.GetString(fieldManager)
	entity.RoleAttributes = bucket.GetStringList(fieldRoleAttributes)
}

func (entity *Employee) SetValues(ctx *PersistContext) {
	ctx.SetString(fieldName, entity.Name)
	ctx.SetStringP(fieldManager, entity.ManagerId)
	ctx.SetStringList(fieldRoleAttributes, entity.RoleAttributes)
}

func (entity *Employee) GetEntityType() string {
	return entityTypeEmployee
}

func newEmployeeStore() *employeeStoreImpl {
	store := &employeeStoreImpl{
		BaseStore: NewBaseStore(entityTypeEmployee, func(id string) error {
			return errors.Errorf("entity of type %v with id %v not found", entityTypeEmployee, id)
		}, "stores"),
	}
	store.InitImpl(store)
	return store
}

type employeeStoreImpl struct {
	*BaseStore
	stores *testStores

	symbolLocations EntitySetSymbol
	indexName       ReadIndex
	indexRoles      SetReadIndex

	locationsCollection LinkCollection
}

func (store *employeeStoreImpl) NewStoreEntity() Entity {
	return &Employee{}
}

func (store *employeeStoreImpl) initializeLocal(constraint bool) {
	store.AddIdSymbol("id", ast.NodeTypeString)
	symbolName := store.AddSymbol(fieldName, ast.NodeTypeString)
	store.indexName = store.AddUniqueIndex(symbolName)

	rolesSymbol := store.AddSetSymbol(fieldRoleAttributes, ast.NodeTypeString)
	store.indexRoles = store.AddSetIndex(rolesSymbol)

	managerSymbol := store.AddFkSymbol(fieldManager, store)

	directReportsSymbol := store.AddFkSetSymbol(fieldDirectReports, store)
	if constraint {
		store.AddFkConstraint(managerSymbol, true, CascadeNone)
	} else {
		store.AddNullableFkIndex(managerSymbol, directReportsSymbol)
	}

	store.symbolLocations = store.AddFkSetSymbol(entityTypeLocation, store.stores.location)

}

func (store *employeeStoreImpl) initializeLinked() {
	store.locationsCollection = store.AddLinkCollection(store.symbolLocations, store.stores.location.symbolEmployees)
}

func (store *employeeStoreImpl) getEmployeesWithRoleAttribute(tx *bbolt.Tx, role string) []string {
	var result []string
	store.indexRoles.Read(tx, []byte(role), func(val []byte) {
		result = append(result, string(val))
	})
	sort.Strings(result)
	return result
}
