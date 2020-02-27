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

package db

import (
	"encoding/binary"
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
	FieldServerPeerData  = "peerdata"
)

type Service struct {
	Id              string
	Binding         string
	EndpointAddress string
	Egress          string
	PeerData        map[uint32][]byte
}

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

	data := bucket.GetBucket(FieldServerPeerData)
	if data != nil {
		service.PeerData = make(map[uint32][]byte)
		iter := data.Cursor()
		for k, v := iter.First(); k != nil; k, v = iter.Next() {
			service.PeerData[binary.LittleEndian.Uint32(k)] = v
		}
	}
}

func (service *Service) SetValues(ctx *boltz.PersistContext) {
	ctx.SetString(FieldServiceBinding, service.Binding)
	ctx.SetString(FieldServiceEndpoint, service.EndpointAddress)
	ctx.SetString(FieldServiceEgress, service.Egress)

	_ = ctx.Bucket.DeleteBucket([]byte(FieldServerPeerData))
	if service.PeerData != nil {
		hostDataBucket := ctx.Bucket.GetOrCreateBucket(FieldServerPeerData)
		for k, v := range service.PeerData {
			key := make([]byte, 4)
			binary.LittleEndian.PutUint32(key, k)
			hostDataBucket.PutValue(key, v)
		}
	}
}

func (service *Service) GetEntityType() string {
	return EntityTypeServices
}

type ServiceStore interface {
	store
	LoadOneById(tx *bbolt.Tx, id string) (*Service, error)
}

func newServiceStore(stores *stores) *serviceStoreImpl {
	notFoundErrorFactory := func(id string) error {
		return fmt.Errorf("missing service '%s'", id)
	}

	store := &serviceStoreImpl{
		baseStore: baseStore{
			stores:    stores,
			BaseStore: boltz.NewBaseStore(nil, EntityTypeServices, notFoundErrorFactory, boltz.RootBucket),
		},
	}
	store.InitImpl(store)
	store.AddSymbol(FieldServiceBinding, ast.NodeTypeString)
	store.AddSymbol(FieldServiceEndpoint, ast.NodeTypeString)
	store.AddSymbol(FieldServiceEgress, ast.NodeTypeString)
	return store
}

type serviceStoreImpl struct {
	baseStore
}

func (store *serviceStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &Service{}
}

func (store *serviceStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Service, error) {
	entity := &Service{}
	if found, err := store.BaseLoadOneById(tx, id, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
