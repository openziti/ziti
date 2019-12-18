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

type EnrollmentUpdbStore interface {
	LoadOneById(id string, pl *Preloads) (*EnrollmentUpdb, error)
	BaseLoadOneByEnrollmentId(enrollId string, pl *Preloads) (BaseDbModel, error)
	LoadOneByEnrollmentId(enrollId string, pl *Preloads) (*EnrollmentUpdb, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*EnrollmentUpdb, error)
	LoadOneByName(name string, pl *Preloads) (*EnrollmentUpdb, error)
	LoadList(qo *QueryOptions) ([]*EnrollmentUpdb, error)
	Create(e *EnrollmentUpdb) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	DeleteWhereWithTx(tx *gorm.DB, p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *EnrollmentUpdb) error
	Patch(e *EnrollmentUpdb) error
	Store
}

type EnrollmentUpdbGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewEnrollmentUpdbGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *EnrollmentUpdbGormStore {
	return &EnrollmentUpdbGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *EnrollmentUpdbGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id": "authenticator_updbs.id",
	}
}

func (igs *EnrollmentUpdbGormStore) EntityName() string {
	return "authenticatorUpdb"
}

func (igs *EnrollmentUpdbGormStore) PluralEntityName() string {
	return "authenticatorUpdbs"
}

func (igs *EnrollmentUpdbGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *EnrollmentUpdb

	if i, ok = e.(*EnrollmentUpdb); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *EnrollmentUpdbGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *EnrollmentUpdbGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *EnrollmentUpdbGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *EnrollmentUpdbGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *EnrollmentUpdbGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)
}

func (igs *EnrollmentUpdbGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *EnrollmentUpdbGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *EnrollmentUpdbGormStore) BaseLoadOneByEnrollmentId(enrollid string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneByEnrollmentId(enrollid, pl)
}

func (igs *EnrollmentUpdbGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *EnrollmentUpdb

	if i, ok = e.(*EnrollmentUpdb); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *EnrollmentUpdbGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *EnrollmentUpdb

	if i, ok = e.(*EnrollmentUpdb); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *EnrollmentUpdbGormStore) LoadOneByEnrollmentId(enrollId string, pl *Preloads) (*EnrollmentUpdb, error) {
	out := &EnrollmentUpdb{}
	p := &predicate.Predicate{
		Clause: squirrel.Eq{"enrollment_id": enrollId},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)

	return out, res.Error
}

func (igs *EnrollmentUpdbGormStore) LoadOneById(id string, pl *Preloads) (*EnrollmentUpdb, error) {
	out := &EnrollmentUpdb{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *EnrollmentUpdbGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*EnrollmentUpdb, error) {
	out := &EnrollmentUpdb{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *EnrollmentUpdbGormStore) LoadOneByName(name string, pl *Preloads) (*EnrollmentUpdb, error) {
	out := &EnrollmentUpdb{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"username": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *EnrollmentUpdbGormStore) LoadList(qo *QueryOptions) ([]*EnrollmentUpdb, error) {
	out := make([]*EnrollmentUpdb, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *EnrollmentUpdbGormStore) Create(e *EnrollmentUpdb) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *EnrollmentUpdbGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &EnrollmentUpdb{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *EnrollmentUpdbGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*EnrollmentUpdb{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *EnrollmentUpdbGormStore) DeleteWhereWithTx(tx *gorm.DB, p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(tx, &[]*EnrollmentUpdb{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *EnrollmentUpdbGormStore) Update(e *EnrollmentUpdb) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *EnrollmentUpdbGormStore) Patch(e *EnrollmentUpdb) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *EnrollmentUpdbGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*EnrollmentUpdb{})
}
