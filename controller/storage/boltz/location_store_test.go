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

func (entity *Location) GetEntityType() string {
	return entityTypeLocation
}

type locationEntityStrategy struct{}

func (self locationEntityStrategy) NewEntity() *Location {
	return new(Location)
}

func (self locationEntityStrategy) FillEntity(*Location, *TypedBucket) {
}

func (self locationEntityStrategy) PersistEntity(*Location, *PersistContext) {
}

func newLocationStore() *locationStoreImpl {
	storeDef := StoreDefinition[*Location]{
		EntityType:     entityTypeLocation,
		EntityStrategy: locationEntityStrategy{},
		EntityNotFoundF: func(id string) error {
			return NewNotFoundError(entityTypeLocation, "id", id)
		},
		BasePath: []string{"stores"},
	}

	store := &locationStoreImpl{
		BaseStore: NewBaseStore(storeDef),
	}
	store.InitImpl(store)
	return store
}

type locationStoreImpl struct {
	*BaseStore[*Location]
	stores              *testStores
	symbolEmployees     EntitySetSymbol
	employeesCollection LinkCollection
}

func (store *locationStoreImpl) NewStoreEntity() *Location {
	return &Location{}
}

func (store *locationStoreImpl) initializeLocal() {
	store.AddIdSymbol("id", ast.NodeTypeString)
	store.symbolEmployees = store.AddFkSetSymbol(entityTypeEmployee, store.stores.employee)
}

func (store *locationStoreImpl) initializeLinked() {
	store.employeesCollection = store.AddLinkCollection(store.symbolEmployees, store.stores.employee.symbolLocations)
}
