/*
	Copyright 2020 NetFoundry, Inc.

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
	EntityTypeTerminators  = "terminators"
	FieldTerminatorService = "service"
	FieldTerminatorRouter  = "router"
	FieldTerminatorBinding = "binding"
	FieldTerminatorAddress = "address"
	FieldServerPeerData    = "peerData"
)

type Terminator struct {
	boltz.BaseExtEntity
	Service  string
	Router   string
	Binding  string
	Address  string
	PeerData map[uint32][]byte
}

func (entity *Terminator) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Service = bucket.GetStringOrError(FieldTerminatorService)
	entity.Router = bucket.GetStringOrError(FieldTerminatorRouter)
	entity.Binding = bucket.GetStringOrError(FieldTerminatorBinding)
	entity.Address = bucket.GetStringWithDefault(FieldTerminatorAddress, "")

	data := bucket.GetBucket(FieldServerPeerData)
	if data != nil {
		entity.PeerData = make(map[uint32][]byte)
		iter := data.Cursor()
		for k, v := iter.First(); k != nil; k, v = iter.Next() {
			entity.PeerData[binary.LittleEndian.Uint32(k)] = v
		}
	}
}

func (entity *Terminator) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldTerminatorService, entity.Service)
	ctx.SetString(FieldTerminatorRouter, entity.Router)
	ctx.SetString(FieldTerminatorBinding, entity.Binding)
	ctx.SetString(FieldTerminatorAddress, entity.Address)

	_ = ctx.Bucket.DeleteBucket([]byte(FieldServerPeerData))
	if entity.PeerData != nil {
		hostDataBucket := ctx.Bucket.GetOrCreateBucket(FieldServerPeerData)
		for k, v := range entity.PeerData {
			key := make([]byte, 4)
			binary.LittleEndian.PutUint32(key, k)
			hostDataBucket.PutValue(key, v)
		}
	}
}

func (entity *Terminator) GetEntityType() string {
	return EntityTypeTerminators
}

type TerminatorStore interface {
	boltz.CrudStore
	LoadOneById(tx *bbolt.Tx, id string) (*Terminator, error)
}

func newTerminatorStore(stores *stores) *terminatorStoreImpl {
	notFoundErrorFactory := func(id string) error {
		return fmt.Errorf("missing terminator '%s'", id)
	}

	store := &terminatorStoreImpl{
		baseStore: baseStore{
			stores:    stores,
			BaseStore: boltz.NewBaseStore(nil, EntityTypeTerminators, notFoundErrorFactory, boltz.RootBucket),
		},
	}
	store.InitImpl(store)
	store.AddExtEntitySymbols()
	store.AddSymbol(FieldTerminatorBinding, ast.NodeTypeString)
	store.AddSymbol(FieldTerminatorAddress, ast.NodeTypeString)

	store.serviceSymbol = store.AddFkSymbol(FieldTerminatorService, store.stores.service)
	store.routerSymbol = store.AddFkSymbol(FieldTerminatorRouter, store.stores.router)

	return store
}

type terminatorStoreImpl struct {
	baseStore

	serviceSymbol boltz.EntitySymbol
	routerSymbol  boltz.EntitySymbol
}

func (store *terminatorStoreImpl) NewStoreEntity() boltz.Entity {
	return &Terminator{}
}

func (store *terminatorStoreImpl) initializeLinked() {
	store.AddFkIndex(store.serviceSymbol, store.stores.service.terminatorsSymbol)
	store.AddFkIndex(store.routerSymbol, store.stores.router.terminatorsSymbol)
}

func (store *terminatorStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Terminator, error) {
	entity := &Terminator{}
	if found, err := store.BaseLoadOneById(tx, id, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
