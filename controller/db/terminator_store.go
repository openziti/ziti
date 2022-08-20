/*
	Copyright NetFoundry Inc.

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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/v2/sequence"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	EntityTypeTerminators         = "terminators"
	FieldTerminatorService        = "service"
	FieldTerminatorRouter         = "router"
	FieldTerminatorBinding        = "binding"
	FieldTerminatorAddress        = "address"
	FieldTerminatorInstanceId     = "instanceId"
	FieldTerminatorInstanceSecret = "instanceSecret"
	FieldTerminatorCost           = "cost"
	FieldTerminatorPrecedence     = "precedence"
	FieldServerPeerData           = "peerData"
	FieldTerminatorHostId         = "hostId"
)

type Terminator struct {
	boltz.BaseExtEntity
	Service        string
	Router         string
	Binding        string
	Address        string
	InstanceId     string
	InstanceSecret []byte
	Cost           uint16
	Precedence     string
	PeerData       xt.PeerData
	HostId         string
}

func (entity *Terminator) GetCost() uint16 {
	return entity.Cost
}

func (entity *Terminator) GetPrecedence() xt.Precedence {
	return xt.GetPrecedenceForName(entity.Precedence)
}

func (entity *Terminator) GetServiceId() string {
	return entity.Service
}

func (entity *Terminator) GetRouterId() string {
	return entity.Router
}

func (entity *Terminator) GetBinding() string {
	return entity.Binding
}

func (entity *Terminator) GetAddress() string {
	return entity.Address
}

func (entity *Terminator) GetInstanceId() string {
	return entity.InstanceId
}

func (entity *Terminator) GetInstanceSecret() []byte {
	return entity.InstanceSecret
}

func (entity *Terminator) GetPeerData() xt.PeerData {
	return entity.PeerData
}

func (entity *Terminator) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Service = bucket.GetStringOrError(FieldTerminatorService)
	entity.Router = bucket.GetStringOrError(FieldTerminatorRouter)
	entity.Binding = bucket.GetStringOrError(FieldTerminatorBinding)
	entity.Address = bucket.GetStringWithDefault(FieldTerminatorAddress, "")
	entity.InstanceId = bucket.GetStringWithDefault(FieldTerminatorInstanceId, "")
	entity.InstanceSecret = bucket.Get([]byte(FieldTerminatorInstanceSecret))
	entity.Cost = uint16(bucket.GetInt32WithDefault(FieldTerminatorCost, 0))
	entity.Precedence = bucket.GetStringWithDefault(FieldTerminatorPrecedence, xt.Precedences.Default.String())
	entity.HostId = bucket.GetStringWithDefault(FieldTerminatorHostId, "")
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

	if entity.Precedence == "" {
		entity.Precedence = xt.Precedences.Default.String()
	}

	terminatorStore := ctx.Store.(*terminatorStoreImpl)

	if ctx.Bucket.HasError() {
		return
	}

	if ctx.IsCreate { // don't allow service, identity or secret to be changed
		ctx.SetRequiredString(FieldTerminatorService, entity.Service)
		ctx.SetString(FieldTerminatorInstanceId, entity.InstanceId)
		if entity.InstanceSecret != nil {
			ctx.Bucket.PutValue([]byte(FieldTerminatorInstanceSecret), entity.InstanceSecret)
		}
	}

	ctx.SetRequiredString(FieldTerminatorRouter, entity.Router)
	ctx.SetRequiredString(FieldTerminatorBinding, entity.Binding)
	ctx.SetRequiredString(FieldTerminatorAddress, entity.Address)
	ctx.SetInt32(FieldTerminatorCost, int32(entity.Cost))
	ctx.SetRequiredString(FieldTerminatorPrecedence, entity.Precedence)
	ctx.SetString(FieldTerminatorHostId, entity.HostId)

	if ctx.ProceedWithSet(FieldServerPeerData) {
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

	if ctx.Bucket.HasError() {
		return
	}

	serviceId := ctx.Bucket.GetStringOrError(FieldTerminatorService) // service won't be passed in on change
	service, err := terminatorStore.stores.service.LoadOneById(ctx.Bucket.Tx(), serviceId)
	if err != nil || service == nil {
		ctx.Bucket.SetError(err)
		return
	}

	strategy, err := xt.GlobalRegistry().GetStrategy(service.TerminatorStrategy)
	ctx.Bucket.SetError(err)

	if ctx.Bucket.HasError() {
		return
	}

	var event xt.StrategyChangeEvent
	terminators, err := terminatorStore.stores.service.getTerminators(ctx.Bucket.Tx(), serviceId)
	ctx.Bucket.SetError(err)
	if ctx.IsCreate {
		event = xt.NewStrategyChangeEvent(entity.Id, terminators, xt.TList(entity), nil, nil)
	} else {
		event = xt.NewStrategyChangeEvent(entity.Id, terminators, nil, xt.TList(entity), nil)
	}
	ctx.Bucket.SetError(strategy.HandleTerminatorChange(event))
}

func (entity *Terminator) GetEntityType() string {
	return EntityTypeTerminators
}

type TerminatorStore interface {
	boltz.CrudStore
	LoadOneById(tx *bbolt.Tx, id string) (*Terminator, error)
	GetTerminatorsInIdentityGroup(tx *bbolt.Tx, terminatorId string) ([]*Terminator, error)
}

func newTerminatorStore(stores *stores) *terminatorStoreImpl {
	notFoundErrorFactory := func(id string) error {
		return boltz.NewNotFoundError(boltz.GetSingularEntityType(EntityTypeTerminators), "id", id)
	}

	store := &terminatorStoreImpl{
		baseStore: baseStore{
			stores:    stores,
			BaseStore: boltz.NewBaseStore(EntityTypeTerminators, notFoundErrorFactory, RootBucket),
		},
		sequence: sequence.NewSequence(),
	}
	store.InitImpl(store)
	return store
}

type terminatorStoreImpl struct {
	baseStore
	sequence *sequence.Sequence

	serviceSymbol boltz.EntitySymbol
	routerSymbol  boltz.EntitySymbol
}

func (store *terminatorStoreImpl) NewStoreEntity() boltz.Entity {
	return &Terminator{}
}

func (store *terminatorStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.AddSymbol(FieldTerminatorBinding, ast.NodeTypeString)
	store.AddSymbol(FieldTerminatorAddress, ast.NodeTypeString)
	store.AddSymbol(FieldTerminatorInstanceId, ast.NodeTypeString)
	store.AddSymbol(FieldTerminatorHostId, ast.NodeTypeString)

	store.serviceSymbol = store.AddFkSymbol(FieldTerminatorService, store.stores.service)
	store.routerSymbol = store.AddFkSymbol(FieldTerminatorRouter, store.stores.router)
}

func (store *terminatorStoreImpl) initializeLinked() {
	store.AddFkIndex(store.serviceSymbol, store.stores.service.terminatorsSymbol)
	store.AddFkIndex(store.routerSymbol, store.stores.router.terminatorsSymbol)

	store.MakeSymbolPublic("service.name")
	store.MakeSymbolPublic("router.name")
}

func (store *terminatorStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Terminator, error) {
	entity := &Terminator{}
	if found, err := store.BaseLoadOneById(tx, id, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *terminatorStoreImpl) Create(ctx boltz.MutateContext, entity boltz.Entity) error {
	if entity.GetId() == "" {
		var err error
		id, err := store.sequence.NextHash()
		if err != nil {
			return err
		}
		entity.SetId(id)
	}
	return store.baseStore.Create(ctx, entity)
}

func (store *terminatorStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	if terminator, err := store.LoadOneById(ctx.Tx(), id); terminator != nil {
		if service, err := store.stores.service.LoadOneById(ctx.Tx(), terminator.Service); service != nil {
			if strategy, err := xt.GlobalRegistry().GetStrategy(service.TerminatorStrategy); strategy != nil {
				if terminators, err := store.stores.service.getTerminators(ctx.Tx(), service.Id); err == nil {
					event := xt.NewStrategyChangeEvent(service.Id, terminators, nil, nil, xt.TList(terminator))
					if err = strategy.HandleTerminatorChange(event); err != nil {
						return err
					}
				} else {
					pfxlog.Logger().Debugf("could not get terminators service %v for terminator %v while deleting terminator (%v)",
						terminator.Service, id, err)
				}
			} else {
				pfxlog.Logger().Debugf("could not find strategy %v on service %v for terminator %v while deleting terminator (%v)",
					service.TerminatorStrategy, terminator.Service, id, err)
			}
		} else {
			pfxlog.Logger().Debugf("could not find service %v for terminator %v while deleting (%v)", terminator.Service, id, err)
		}
	} else {
		pfxlog.Logger().Debugf("could not find terminator %v for delete (%v)", id, err)
	}

	return store.baseStore.DeleteById(ctx, id)
}

func (store *terminatorStoreImpl) GetTerminatorsInIdentityGroup(tx *bbolt.Tx, terminatorId string) ([]*Terminator, error) {
	terminator, err := store.LoadOneById(tx, terminatorId)
	if err != nil {
		return nil, err
	}
	if terminator == nil {
		return nil, boltz.NewNotFoundError("terminator", "id", terminatorId)
	}

	serviceId := terminator.GetServiceId()
	identity := terminator.GetInstanceId()

	terminatorIds := store.stores.service.GetRelatedEntitiesIdList(tx, serviceId, EntityTypeTerminators)
	var identityTerminators []*Terminator
	for _, siblingId := range terminatorIds {
		if siblingId != terminatorId {
			if terminator, _ := store.LoadOneById(tx, siblingId); terminator != nil {
				if identity == terminator.InstanceId {
					identityTerminators = append(identityTerminators, terminator)
				}
			}
		}
	}
	return identityTerminators, nil
}
