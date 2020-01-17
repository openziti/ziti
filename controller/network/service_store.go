/*
	Copyright 2019 NetFoundry, Inc.

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

package network

import (
	"fmt"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	EntityTypeServices   = "services"
	FieldServiceBinding  = "binding"
	FieldServiceEndpoint = "endpoint"
	FieldServiceEgress   = "egress"
)

func (service *Service) GetId() string {
	return service.Id
}

func (service *Service) SetId(id string) {
	service.Id = id
}

func (service *Service) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	service.Binding = bucket.GetStringWithDefault(FieldServiceBinding, "")
	service.EndpointAddress = bucket.GetStringWithDefault(FieldServiceEndpoint, "")
	service.Egress = bucket.GetStringWithDefault(FieldServiceEgress, "")
}

func (service *Service) SetValues(ctx *boltz.PersistContext) {
	ctx.SetString(FieldServiceBinding, service.Binding)
	ctx.SetString(FieldServiceEndpoint, service.EndpointAddress)
	ctx.SetString(FieldServiceEgress, service.Egress)
}

func (service *Service) GetEntityType() string {
	return EntityTypeServices
}

type ServiceStore interface {
	boltz.CrudStore
	create(service *Service) error
	update(svc *Service) error
	remove(id string) error
	all() ([]*Service, error)
	loadOneById(id string) (*Service, error)
	LoadOneById(tx *bbolt.Tx, id string) (*Service, error)
}

func NewServiceStore(db boltz.Db) ServiceStore {
	notFoundErrorFactory := func(id string) error {
		return fmt.Errorf("missing service '%s'", id)
	}

	store := &serviceStoreImpl{
		db:        db,
		BaseStore: boltz.NewBaseStore(nil, EntityTypeServices, notFoundErrorFactory, boltz.RootBucket),
	}
	store.InitImpl(store)
	store.AddSymbol(FieldServiceBinding, ast.NodeTypeString)
	store.AddSymbol(FieldServiceEndpoint, ast.NodeTypeString)
	store.AddSymbol(FieldServiceEgress, ast.NodeTypeString)
	return store
}

type serviceStoreImpl struct {
	db boltz.Db
	*boltz.BaseStore
}

func (store *serviceStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &Service{}
}

func (store *serviceStoreImpl) create(service *Service) error {
	return store.db.Update(func(tx *bbolt.Tx) error {
		return store.Create(boltz.NewMutateContext(tx), service)
	})
}

func (store *serviceStoreImpl) update(service *Service) error {
	return store.db.Update(func(tx *bbolt.Tx) error {
		return store.Update(boltz.NewMutateContext(tx), service, nil)
	})
}

func (store *serviceStoreImpl) remove(id string) error {
	return store.db.Update(func(tx *bbolt.Tx) error {
		return store.DeleteById(boltz.NewMutateContext(tx), id)
	})
}

func (store *serviceStoreImpl) loadOneById(id string) (service *Service, err error) {
	err = store.db.View(func(tx *bbolt.Tx) error {
		service, err = store.LoadOneById(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, fmt.Errorf("missing service '%s'", id)
	}
	return
}

func (store *serviceStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Service, error) {
	service := &Service{}
	if found, err := store.BaseLoadOneById(tx, id, service); !found || err != nil {
		return nil, err
	}
	return service, nil
}

func (store *serviceStoreImpl) all() ([]*Service, error) {
	services := make([]*Service, 0)
	err := store.db.View(func(tx *bbolt.Tx) error {
		ids, _, err := store.QueryIds(tx, "true")
		if err != nil {
			return err
		}
		for _, id := range ids {
			service, err := store.LoadOneById(tx, string(id))
			if err != nil {
				return err
			}
			services = append(services, service)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return services, nil
}