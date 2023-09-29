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
	EntityTypeTerminators          = "terminators"
	FieldTerminatorService         = "service"
	FieldTerminatorRouter          = "router"
	FieldTerminatorBinding         = "binding"
	FieldTerminatorAddress         = "address"
	FieldTerminatorInstanceId      = "instanceId"
	FieldTerminatorInstanceSecret  = "instanceSecret"
	FieldTerminatorCost            = "cost"
	FieldTerminatorPrecedence      = "precedence"
	FieldServerPeerData            = "peerData"
	FieldTerminatorHostId          = "hostId"
	FieldTerminatorSavedPrecedence = "savedPrecedence"
)

type Terminator struct {
	boltz.BaseExtEntity
	Service         string      `json:"service"`
	Router          string      `json:"router"`
	Binding         string      `json:"binding"`
	Address         string      `json:"address"`
	InstanceId      string      `json:"instanceId"`
	InstanceSecret  []byte      `json:"instanceSecret"`
	Cost            uint16      `json:"cost"`
	Precedence      string      `json:"precedence"`
	PeerData        xt.PeerData `json:"peerData"`
	HostId          string      `json:"hostId"`
	SavedPrecedence *string     `json:"savedPrecedence"`
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

func (entity *Terminator) GetHostId() string {
	return entity.HostId
}

func (entity *Terminator) GetEntityType() string {
	return EntityTypeTerminators
}

type TerminatorStore interface {
	boltz.EntityStore[*Terminator]
	GetTerminatorsInIdentityGroup(tx *bbolt.Tx, terminatorId string) ([]*Terminator, error)
}

func newTerminatorStore(stores *stores) *terminatorStoreImpl {
	store := &terminatorStoreImpl{
		sequence: sequence.NewSequence(),
	}

	store.baseStore = baseStore[*Terminator]{
		stores:    stores,
		BaseStore: boltz.NewBaseStore(NewStoreDefinition[*Terminator](store)),
	}

	store.InitImpl(store)
	return store
}

type terminatorStoreImpl struct {
	baseStore[*Terminator]
	sequence *sequence.Sequence

	serviceSymbol boltz.EntitySymbol
	routerSymbol  boltz.EntitySymbol
}

func (store *terminatorStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.AddSymbol(FieldTerminatorBinding, ast.NodeTypeString)
	store.AddSymbol(FieldTerminatorAddress, ast.NodeTypeString)
	store.AddSymbol(FieldTerminatorInstanceId, ast.NodeTypeString)
	store.AddSymbol(FieldTerminatorHostId, ast.NodeTypeString)

	store.serviceSymbol = store.AddFkSymbol(FieldTerminatorService, store.stores.service)
	store.routerSymbol = store.AddFkSymbol(FieldTerminatorRouter, store.stores.router)

	store.AddConstraint(boltz.NewSystemEntityEnforcementConstraint(store))
}

func (store *terminatorStoreImpl) initializeLinked() {
	store.AddFkIndex(store.serviceSymbol, store.stores.service.terminatorsSymbol)
	store.AddFkIndex(store.routerSymbol, store.stores.router.terminatorsSymbol)

	store.MakeSymbolPublic("service.name")
	store.MakeSymbolPublic("router.name")
}

func (store *terminatorStoreImpl) NewEntity() *Terminator {
	return &Terminator{}
}

func (store *terminatorStoreImpl) FillEntity(entity *Terminator, bucket *boltz.TypedBucket) {
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
	entity.SavedPrecedence = bucket.GetString(FieldTerminatorSavedPrecedence)

	data := bucket.GetBucket(FieldServerPeerData)
	if data != nil {
		entity.PeerData = make(map[uint32][]byte)
		iter := data.Cursor()
		for k, v := iter.First(); k != nil; k, v = iter.Next() {
			entity.PeerData[binary.LittleEndian.Uint32(k)] = v
		}
	}
}

func (store *terminatorStoreImpl) PersistEntity(entity *Terminator, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)

	if entity.Precedence == "" {
		entity.Precedence = xt.Precedences.Default.String()
	}

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
	ctx.SetStringP(FieldTerminatorSavedPrecedence, entity.SavedPrecedence)

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
	service, _, err := store.stores.service.FindById(ctx.Bucket.Tx(), serviceId)
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
	terminators, err := store.stores.service.getTerminators(ctx.Bucket.Tx(), serviceId)
	ctx.Bucket.SetError(err)
	if ctx.IsCreate {
		event = xt.NewStrategyChangeEvent(entity.Id, terminators, xt.TList(entity), nil, nil)
	} else {
		event = xt.NewStrategyChangeEvent(entity.Id, terminators, nil, xt.TList(entity), nil)
	}
	ctx.Bucket.SetError(strategy.HandleTerminatorChange(event))
}

func (store *terminatorStoreImpl) Create(ctx boltz.MutateContext, entity *Terminator) error {
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
	ctx = ctx.GetSystemContext()
	if terminator, _, _ := store.FindById(ctx.Tx(), id); terminator != nil {
		if service, _, err := store.stores.service.FindById(ctx.Tx(), terminator.Service); service != nil {
			if strategy, err := xt.GlobalRegistry().GetStrategy(service.TerminatorStrategy); strategy != nil {
				if terminators, err := store.stores.service.getTerminators(ctx.Tx(), service.Id); err == nil {
					event := xt.NewStrategyChangeEvent(service.Id, terminators, nil, nil, xt.TList(terminator))
					if err = strategy.HandleTerminatorChange(event); err != nil {
						return err
					}
				} else {
					pfxlog.Logger().WithError(err).Errorf("could not get terminators service %v for terminator %v while deleting terminator",
						terminator.Service, id)
				}
			} else {
				pfxlog.Logger().WithError(err).Errorf("could not find strategy %v on service %v for terminator %v while deleting terminator",
					service.TerminatorStrategy, terminator.Service, id)
			}
		} else {
			pfxlog.Logger().WithError(err).Errorf("could not find service %v for terminator %v while deleting", terminator.Service, id)
		}
	}

	return store.baseStore.DeleteById(ctx, id)
}

func (store *terminatorStoreImpl) GetTerminatorsInIdentityGroup(tx *bbolt.Tx, terminatorId string) ([]*Terminator, error) {
	terminator, _, err := store.FindById(tx, terminatorId)
	if err != nil {
		return nil, err
	}
	if terminator == nil {
		return nil, boltz.NewNotFoundError("terminator", "id", terminatorId)
	}

	serviceId := terminator.GetServiceId()

	terminatorIds := store.stores.service.GetRelatedEntitiesIdList(tx, serviceId, EntityTypeTerminators)
	var identityTerminators []*Terminator
	for _, siblingId := range terminatorIds {
		if siblingId != terminatorId {
			if sibling, _, _ := store.FindById(tx, siblingId); sibling != nil {
				if terminator.InstanceId == sibling.InstanceId {
					identityTerminators = append(identityTerminators, sibling)
				}
			}
		}
	}
	return identityTerminators, nil
}
