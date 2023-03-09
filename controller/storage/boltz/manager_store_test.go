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

type Manager struct {
	Employee
	IsTechLead bool
}

type managerEntityStrategy struct {
	employeeStore *employeeStoreImpl
}

func (self *managerEntityStrategy) New() *Manager {
	return new(Manager)
}

func (self *managerEntityStrategy) LoadEntity(entity *Manager, bucket *TypedBucket) {
	_, err := self.employeeStore.LoadEntity(bucket.Tx(), entity.Id, &entity.Employee)
	bucket.SetError(err)
	entity.IsTechLead = bucket.GetBoolWithDefault("isTechLead", false)
}

func (self *managerEntityStrategy) PersistEntity(entity *Manager, ctx *PersistContext) {
	self.employeeStore.GetEntityStrategy().PersistEntity(&entity.Employee, ctx.GetParentContext())
	ctx.SetBool("isTechLead", entity.IsTechLead)
}

func newManagerStore(parent *employeeStoreImpl) *managerStoreImpl {
	storeDef := StoreDefinition[*Manager]{
		EntityStrategy: &managerEntityStrategy{
			employeeStore: parent,
		},
		EntityNotFoundF: func(id string) error {
			return NewNotFoundError(parent.GetSingularEntityType(), "id", id)
		},
		BasePath: []string{"ext"},
		Parent:   parent,
		ParentMapper: func(entity Entity) Entity {
			if mgr, ok := entity.(*Manager); ok {
				return &mgr.Employee
			}
			return entity
		},
	}

	store := &managerStoreImpl{
		BaseStore: NewBaseStore(storeDef),
	}
	store.InitImpl(store)
	return store
}

type managerStoreImpl struct {
	*BaseStore[*Manager]
}

func (store *managerStoreImpl) initializeLocal() {
	store.GetParentStore().GrantSymbols(store)
	store.AddSymbol("isTechLead", ast.NodeTypeBool)
}

func (store *managerStoreImpl) initializeLinked() {
	// does nothing
}
