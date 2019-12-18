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

type CaStore interface {
	LoadOneById(id string, pl *Preloads) (*Ca, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*Ca, error)
	LoadOneByName(name string, pl *Preloads) (*Ca, error)
	LoadList(qo *QueryOptions) ([]*Ca, error)
	Create(e *Ca) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *Ca) error
	Patch(e *Ca) error
	Store
}

type CaGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewCaGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *CaGormStore {
	return &CaGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *CaGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":                        "cas.id",
		"name":                      "cas.name",
		"fingerprint":               "cas.fingerprint",
		"isVerified":                "cas.is_verified",
		"verificationToken":         "cas.verification_token",
		"isAutoCaEnrollmentEnabled": "cas.is_auto_ca_enrollment_enabled",
		"isOttCaEnrollmentEnabled":  "cas.is_ott_ca_enrollment_enabled",
		"isAuthEnabled":             "cas.is_auth_enabled",
		"createdAt":                 "cas.created_at",
		"updatedAt":                 "cas.updated_at",
	}
}

func (igs *CaGormStore) EntityName() string {
	return "ca"
}

func (igs *CaGormStore) PluralEntityName() string {
	return "cas"
}

func (igs *CaGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *Ca

	if i, ok = e.(*Ca); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *CaGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *CaGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *CaGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *CaGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *CaGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *CaGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *CaGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *CaGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *Ca

	if i, ok = e.(*Ca); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *CaGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *Ca

	if i, ok = e.(*Ca); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *CaGormStore) LoadOneById(id string, pl *Preloads) (*Ca, error) {
	out := &Ca{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *CaGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*Ca, error) {
	out := &Ca{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *CaGormStore) LoadOneByName(name string, pl *Preloads) (*Ca, error) {
	out := &Ca{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *CaGormStore) LoadList(qo *QueryOptions) ([]*Ca, error) {
	out := make([]*Ca, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *CaGormStore) Create(e *Ca) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *CaGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &Ca{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *CaGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*Ca{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *CaGormStore) Update(e *Ca) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *CaGormStore) Patch(e *Ca) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *CaGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*Ca{})
}
