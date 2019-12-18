/*
	Copyright 2019 Netfoundry, Inc.

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

package migration

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/kataras/go-events"
	"github.com/netfoundry/ziti-edge/controller/predicate"
	"gopkg.in/Masterminds/squirrel.v1"
	"reflect"
)

type NetworkSessionStore interface {
	LoadOneById(id string, pl *Preloads) (*NetworkSession, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*NetworkSession, error)
	LoadOneByName(name string, pl *Preloads) (*NetworkSession, error)
	LoadList(qo *QueryOptions) ([]*NetworkSession, error)
	LoadListByClusterId(id string, qo *QueryOptions, pl *Preloads) ([]*NetworkSession, error)
	Create(e *NetworkSession) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *NetworkSession) error
	Patch(e *NetworkSession) error
	NewIdentityFilteredStore(i *Identity) NetworkSessionStore
	Store
}

type NetworkSessionGormStore struct {
	db                   *gorm.DB
	dbWithPreloading     *gorm.DB
	filterIdentity       *Identity
	isFilteredByIdentity bool
	events.EventEmmiter
}

//@todo this feels bad, need a way to surface the interface and ensure there is only one model store type initialized
var globalNetworkSessionGormStore *NetworkSessionGormStore

func getDefaultNetworkSessionGormStore() *NetworkSessionGormStore {
	return globalNetworkSessionGormStore
}

func NewNetworkSessionGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *NetworkSessionGormStore {
	ns := &NetworkSessionGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}

	//@todo this feels bad, need a way to surface the interface and ensure there is only one model store type initialized
	if globalNetworkSessionGormStore == nil {
		globalNetworkSessionGormStore = ns
	}

	return ns
}

func (igs *NetworkSessionGormStore) NewIdentityFilteredStore(i *Identity) NetworkSessionStore {
	figs := NewNetworkSessionGormStore(igs.db, igs.dbWithPreloading)
	figs.EventEmmiter = igs.EventEmmiter
	figs.isFilteredByIdentity = true
	figs.filterIdentity = i

	return figs
}

func (igs *NetworkSessionGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":        "network_sessions.id",
		"sessionId": "network_sessions.session_id",
		"serviceId": "network_sessions.service_id",
		"hosting":   "network_sessions.hosting",
		"createdAt": "network_sessions.created_at",
		"updatedAt": "network_sessions.updated_at",
	}
}

func (igs *NetworkSessionGormStore) EntityName() string {
	return "networkSession"
}

func (igs *NetworkSessionGormStore) PluralEntityName() string {
	return "networkSessions"
}

func (igs *NetworkSessionGormStore) BaseCreate(e BaseDbModel) (string, error) {
	i, ok := e.(*NetworkSession)
	if !ok {
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> NetworkSession", reflect.TypeOf(e).Name())
	}
	return igs.Create(i)
}

func (igs *NetworkSessionGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *NetworkSessionGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *NetworkSessionGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *NetworkSessionGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
	is, err := igs.LoadList(qo)

	if err != nil {
		return nil, err
	}

	var bms []BaseDbModel

	for _, i := range is {
		bms = append(bms, i)
	}

	return bms, nil
}
func (igs *NetworkSessionGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *NetworkSessionGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *NetworkSessionGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *NetworkSessionGormStore) BaseUpdate(e BaseDbModel) error {
	i, ok := e.(*NetworkSession)
	if !ok {
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> NetworkSession", reflect.TypeOf(e).Name())
	}
	return igs.Update(i)
}

func (igs *NetworkSessionGormStore) BasePatch(e BaseDbModel) error {
	i, ok := e.(*NetworkSession)
	if !ok {
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> NetworkSession", reflect.TypeOf(e).Name())
	}
	return igs.Patch(i)
}

func (igs *NetworkSessionGormStore) LoadOneById(id string, pl *Preloads) (*NetworkSession, error) {
	p := &predicate.Predicate{
		Clause: squirrel.Eq{"network_sessions.ID": id},
	}
	return igs.LoadOne(p, pl)
}

func (igs *NetworkSessionGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*NetworkSession, error) {
	out := &NetworkSession{}
	rp := p

	if igs.isFilteredByIdentity {
		rp = &predicate.Predicate{
			Callback: func(q *gorm.DB) *gorm.DB {
				q = q.
					Table("network_sessions").
					Select("network_sessions.*").
					Joins("inner join sessions on sessions.id = network_sessions.session_id and sessions.identity_id = ?", igs.filterIdentity.ID)

				if p != nil {
					q = p.Apply(q)
				}

				return q
			},
		}
	}

	res := LoadOne(igs.dbWithPreloading, rp, pl, out)
	return out, res.Error
}

func (igs *NetworkSessionGormStore) LoadOneByName(name string, pl *Preloads) (*NetworkSession, error) {
	out := &NetworkSession{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *NetworkSessionGormStore) LoadListByClusterId(id string, qo *QueryOptions, pl *Preloads) ([]*NetworkSession, error) {
	out := make([]*NetworkSession, qo.Paging.Limit)

	op := qo.Predicate
	qo.Predicate = &predicate.Predicate{
		Callback: func(q *gorm.DB) *gorm.DB {
			q = q.
				Table("network_sessions").
				Select("network_sessions.*").
				Joins("inner join service_clusters on service_clusters.service_id = network_sessions.service_id and service_clusters.cluster_id = ?", id).
				Joins("inner join clusters on clusters.id = service_clusters.cluster_id")
			if op != nil {
				q = op.Apply(q)
			}
			return q
		},
	}

	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *NetworkSessionGormStore) LoadList(qo *QueryOptions) ([]*NetworkSession, error) {
	out := make([]*NetworkSession, qo.Paging.Limit)

	if igs.isFilteredByIdentity {
		op := qo.Predicate
		qo.Predicate = &predicate.Predicate{
			Callback: func(q *gorm.DB) *gorm.DB {
				q = q.
					Table("network_sessions").
					Select("network_sessions.*").
					Joins("inner join sessions on sessions.id = network_sessions.session_id and sessions.identity_id = ?", igs.filterIdentity.ID)

				if op != nil {
					q = op.Apply(q)
				}

				return q
			},
		}
	}

	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *NetworkSessionGormStore) Create(e *NetworkSession) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *NetworkSessionGormStore) DeleteWithTx(e *NetworkSession, tx *gorm.DB) error {
	res := Delete(tx, e, igs)

	return res.Error
}

func (igs *NetworkSessionGormStore) DeleteById(id string) error {
	res := Delete(igs.dbWithPreloading, &NetworkSession{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *NetworkSessionGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.dbWithPreloading, &[]*NetworkSession{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *NetworkSessionGormStore) Update(e *NetworkSession) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *NetworkSessionGormStore) Patch(e *NetworkSession) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *NetworkSessionGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*NetworkSession{})
}
