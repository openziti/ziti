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

type EnrollmentCertStore interface {
	LoadOneById(id string, pl *Preloads) (*EnrollmentCert, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*EnrollmentCert, error)
	LoadOneByName(name string, pl *Preloads) (*EnrollmentCert, error)
	BaseLoadOneByEnrollmentId(enrollId string, pl *Preloads) (BaseDbModel, error)
	LoadOneByEnrollmentId(enrollId string, pl *Preloads) (*EnrollmentCert, error)
	LoadList(qo *QueryOptions) ([]*EnrollmentCert, error)
	Create(e *EnrollmentCert) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	DeleteWhereWithTx(tx *gorm.DB, p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *EnrollmentCert) error
	Patch(e *EnrollmentCert) error
	Store
}

type EnrollmentCertGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewEnrollmentCertGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *EnrollmentCertGormStore {
	return &EnrollmentCertGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *EnrollmentCertGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id": "authenticator_certs.id",
	}
}

func (igs *EnrollmentCertGormStore) EntityName() string {
	return "enrollmentCerts"
}

func (igs *EnrollmentCertGormStore) PluralEntityName() string {
	return "enrollmentCerts"
}

func (igs *EnrollmentCertGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *EnrollmentCert

	if i, ok = e.(*EnrollmentCert); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *EnrollmentCertGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *EnrollmentCertGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *EnrollmentCertGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *EnrollmentCertGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *EnrollmentCertGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *EnrollmentCertGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *EnrollmentCertGormStore) BaseLoadOneByEnrollmentId(enrollId string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneByEnrollmentId(enrollId, pl)
}

func (igs *EnrollmentCertGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *EnrollmentCert

	if i, ok = e.(*EnrollmentCert); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *EnrollmentCertGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *EnrollmentCert

	if i, ok = e.(*EnrollmentCert); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *EnrollmentCertGormStore) LoadOneById(id string, pl *Preloads) (*EnrollmentCert, error) {
	out := &EnrollmentCert{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *EnrollmentCertGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*EnrollmentCert, error) {
	out := &EnrollmentCert{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *EnrollmentCertGormStore) LoadOneByEnrollmentId(enrollId string, pl *Preloads) (*EnrollmentCert, error) {
	out := &EnrollmentCert{}
	p := &predicate.Predicate{
		Clause: squirrel.Eq{"enrollment_id": enrollId},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)

	return out, res.Error
}

func (igs *EnrollmentCertGormStore) LoadOneByName(name string, pl *Preloads) (*EnrollmentCert, error) {
	out := &EnrollmentCert{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *EnrollmentCertGormStore) LoadList(qo *QueryOptions) ([]*EnrollmentCert, error) {
	out := make([]*EnrollmentCert, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *EnrollmentCertGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)
}

func (igs *EnrollmentCertGormStore) Create(e *EnrollmentCert) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *EnrollmentCertGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &EnrollmentCert{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *EnrollmentCertGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*EnrollmentCert{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *EnrollmentCertGormStore) DeleteWhereWithTx(tx *gorm.DB, p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(tx, &[]*EnrollmentCert{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *EnrollmentCertGormStore) Update(e *EnrollmentCert) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *EnrollmentCertGormStore) Patch(e *EnrollmentCert) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *EnrollmentCertGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*EnrollmentCert{})
}
