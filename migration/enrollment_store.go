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

type EnrollmentStore interface {
	LoadOneById(id string, pl *Preloads) (*Enrollment, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*Enrollment, error)
	LoadOneByName(name string, pl *Preloads) (*Enrollment, error)
	LoadOneByToken(token string, pl *Preloads) (*Enrollment, error)
	LoadList(qo *QueryOptions) ([]*Enrollment, error)
	Create(e *Enrollment) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *Enrollment) error
	Patch(e *Enrollment) error
	Store
}

type EnrollmentGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewEnrollmentGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *EnrollmentGormStore {
	return &EnrollmentGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *EnrollmentGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":            "enrollments.id",
		"name":          "enrollments.method",
		"expiresAt":     "enrollments.expires_at",
		"createdAt":     "enrollments.created_at",
		"updatedAt":     "enrollments.updated_at",
		"identity.name": "identities.name",
		"identity.id":   "identities.id",
	}
}

func (igs *EnrollmentGormStore) EntityName() string {
	return "enrollment"
}

func (igs *EnrollmentGormStore) PluralEntityName() string {
	return "enrollments"
}

func (igs *EnrollmentGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *Enrollment

	if i, ok = e.(*Enrollment); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *EnrollmentGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *EnrollmentGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *EnrollmentGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *EnrollmentGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *EnrollmentGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *EnrollmentGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *EnrollmentGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *EnrollmentGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *Enrollment

	if i, ok = e.(*Enrollment); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *EnrollmentGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *Enrollment

	if i, ok = e.(*Enrollment); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *EnrollmentGormStore) LoadOneById(id string, pl *Preloads) (*Enrollment, error) {
	out := &Enrollment{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *EnrollmentGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*Enrollment, error) {
	out := &Enrollment{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *EnrollmentGormStore) LoadOneByName(name string, pl *Preloads) (*Enrollment, error) {
	out := &Enrollment{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *EnrollmentGormStore) LoadOneByToken(token string, pl *Preloads) (*Enrollment, error) {
	out := &Enrollment{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"token": token},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *EnrollmentGormStore) LoadList(qo *QueryOptions) ([]*Enrollment, error) {
	out := make([]*Enrollment, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *EnrollmentGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*Enrollment{})
}

func (igs *EnrollmentGormStore) Create(e *Enrollment) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *EnrollmentGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &Enrollment{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *EnrollmentGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*Enrollment{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *EnrollmentGormStore) Update(e *Enrollment) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *EnrollmentGormStore) Patch(e *Enrollment) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}
