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

type AppWanStore interface {
	LoadOneById(id string, pl *Preloads) (*AppWan, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*AppWan, error)
	LoadOneByName(name string, pl *Preloads) (*AppWan, error)
	LoadList(qo *QueryOptions) ([]*AppWan, error)
	Create(e *AppWan) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *AppWan) error
	Patch(e *AppWan) error
	Store
}

type AppWanGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewAppWanGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *AppWanGormStore {
	return &AppWanGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *AppWanGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":        "app_wans.id",
		"name":      "app_wans.name",
		"createdAt": "app_wans.created_at",
		"updatedAt": "app_wans.updated_at",
	}
}

func (igs *AppWanGormStore) EntityName() string {
	return "appWan"
}

func (igs *AppWanGormStore) PluralEntityName() string {
	return "appWans"
}

func (igs *AppWanGormStore) BaseCreate(e BaseDbModel) (string, error) {
	i, ok := e.(*AppWan)
	if !ok {
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> AppWan", reflect.TypeOf(e).Name())
	}
	return igs.Create(i)
}

func (igs *AppWanGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *AppWanGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *AppWanGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *AppWanGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *AppWanGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *AppWanGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *AppWanGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *AppWanGormStore) BaseUpdate(e BaseDbModel) error {
	i, ok := e.(*AppWan)
	if !ok {
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> AppWan", reflect.TypeOf(e).Name())
	}
	return igs.Update(i)
}

func (igs *AppWanGormStore) BasePatch(e BaseDbModel) error {
	i, ok := e.(*AppWan)
	if !ok {
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> AppWan", reflect.TypeOf(e).Name())
	}
	return igs.Patch(i)
}

func (igs *AppWanGormStore) LoadOneById(id string, pl *Preloads) (*AppWan, error) {
	out := &AppWan{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *AppWanGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*AppWan, error) {
	out := &AppWan{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *AppWanGormStore) LoadOneByName(name string, pl *Preloads) (*AppWan, error) {
	out := &AppWan{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *AppWanGormStore) LoadList(qo *QueryOptions) ([]*AppWan, error) {
	out := make([]*AppWan, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *AppWanGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*AppWan{})
}

func (igs *AppWanGormStore) Create(e *AppWan) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *AppWanGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &AppWan{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *AppWanGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*AppWan{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *AppWanGormStore) Update(e *AppWan) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *AppWanGormStore) Patch(e *AppWan) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}
