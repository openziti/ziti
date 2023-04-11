/*
	Copyright NetFoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package boltz

import (
	"github.com/openziti/storage/ast"
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

func (entity *Employee) GetEntityType() string {
	return entityTypeEmployee
}

type employeeEntityStrategy struct{}

func (self employeeEntityStrategy) NewEntity() *Employee {
	return new(Employee)
}

func (self employeeEntityStrategy) FillEntity(entity *Employee, bucket *TypedBucket) {
	entity.Name = bucket.GetStringOrError(fieldName)
	entity.ManagerId = bucket.GetString(fieldManager)
	entity.RoleAttributes = bucket.GetStringList(fieldRoleAttributes)
}

func (self employeeEntityStrategy) PersistEntity(entity *Employee, ctx *PersistContext) {
	ctx.SetString(fieldName, entity.Name)
	ctx.SetStringP(fieldManager, entity.ManagerId)
	ctx.SetStringList(fieldRoleAttributes, entity.RoleAttributes)
}

func newEmployeeStore() *employeeStoreImpl {
	storeDef := StoreDefinition[*Employee]{
		EntityType:     entityTypeEmployee,
		EntityStrategy: employeeEntityStrategy{},
		EntityNotFoundF: func(id string) error {
			return NewNotFoundError(entityTypeEmployee, "id", id)
		},
		BasePath: []string{"stores"},
	}
	store := &employeeStoreImpl{
		BaseStore: NewBaseStore(storeDef),
	}
	store.InitImpl(store)
	return store
}

type employeeStoreImpl struct {
	*BaseStore[*Employee]
	stores *testStores

	symbolLocations EntitySetSymbol
	indexName       ReadIndex
	indexRoles      SetReadIndex

	locationsCollection LinkCollection
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
