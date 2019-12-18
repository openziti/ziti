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

type ClusterStore interface {
	LoadOneById(id string, pl *Preloads) (*Cluster, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*Cluster, error)
	LoadOneByName(name string, pl *Preloads) (*Cluster, error)
	LoadList(qo *QueryOptions) ([]*Cluster, error)
	Create(e *Cluster) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *Cluster) error
	Patch(e *Cluster) error
	Store
}

type ClusterGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewClusterGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *ClusterGormStore {
	return &ClusterGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *ClusterGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":        "clusters.id",
		"name":      "clusters.name",
		"createdAt": "clusters.created_at",
		"updatedAt": "clusters.updated_at",
	}
}
func (igs *ClusterGormStore) EntityName() string {
	return "cluster"
}

func (igs *ClusterGormStore) PluralEntityName() string {
	return "clusters"
}
func (igs *ClusterGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *Cluster

	if i, ok = e.(*Cluster); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *ClusterGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *ClusterGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *ClusterGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *ClusterGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *ClusterGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *ClusterGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *ClusterGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *ClusterGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *Cluster

	if i, ok = e.(*Cluster); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *ClusterGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *Cluster

	if i, ok = e.(*Cluster); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *ClusterGormStore) LoadOneById(id string, pl *Preloads) (*Cluster, error) {
	out := &Cluster{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *ClusterGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*Cluster, error) {
	out := &Cluster{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *ClusterGormStore) LoadOneByName(name string, pl *Preloads) (*Cluster, error) {
	out := &Cluster{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *ClusterGormStore) LoadList(qo *QueryOptions) ([]*Cluster, error) {
	out := make([]*Cluster, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *ClusterGormStore) Create(e *Cluster) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *ClusterGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &Cluster{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *ClusterGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*Cluster{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *ClusterGormStore) Update(e *Cluster) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *ClusterGormStore) Patch(e *Cluster) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *ClusterGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*Cluster{})
}
