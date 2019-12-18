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

type EnrollStore interface {
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

type AuthStore interface {
	LoadOneById(id string, pl *Preloads) (*AuthenticatorCert, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*AuthenticatorCert, error)
	LoadOneByName(name string, pl *Preloads) (*AuthenticatorCert, error)
	BaseLoadOneByAuthenticatorId(authId string, pl *Preloads) (BaseDbModel, error)
	LoadOneByAuthenticatorId(authId string, pl *Preloads) (*AuthenticatorCert, error)
	LoadList(qo *QueryOptions) ([]*AuthenticatorCert, error)
	Create(e *AuthenticatorCert) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	DeleteWhereWithTx(tx *gorm.DB, p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *AuthenticatorCert) error
	Patch(e *AuthenticatorCert) error
	Store
}

type AuthenticatorCertGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewAuthenticatorCertGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *AuthenticatorCertGormStore {
	return &AuthenticatorCertGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *AuthenticatorCertGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id": "authenticator_certs.id",
	}
}

func (igs *AuthenticatorCertGormStore) EntityName() string {
	return "authenticatorCert"
}

func (igs *AuthenticatorCertGormStore) PluralEntityName() string {
	return "authenticatorCerts"
}

func (igs *AuthenticatorCertGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *AuthenticatorCert

	if i, ok = e.(*AuthenticatorCert); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *AuthenticatorCertGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *AuthenticatorCertGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *AuthenticatorCertGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *AuthenticatorCertGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *AuthenticatorCertGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)
}

func (igs *AuthenticatorCertGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *AuthenticatorCertGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *AuthenticatorCertGormStore) BaseLoadOneByAuthenticatorId(authId string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneByAuthenticatorId(authId, pl)
}

func (igs *AuthenticatorCertGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *AuthenticatorCert

	if i, ok = e.(*AuthenticatorCert); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *AuthenticatorCertGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *AuthenticatorCert

	if i, ok = e.(*AuthenticatorCert); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *AuthenticatorCertGormStore) LoadOneById(id string, pl *Preloads) (*AuthenticatorCert, error) {
	out := &AuthenticatorCert{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *AuthenticatorCertGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*AuthenticatorCert, error) {
	out := &AuthenticatorCert{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *AuthenticatorCertGormStore) LoadOneByAuthenticatorId(authId string, pl *Preloads) (*AuthenticatorCert, error) {
	out := &AuthenticatorCert{}
	p := &predicate.Predicate{
		Clause: squirrel.Eq{"authenticator_id": authId},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)

	return out, res.Error
}

func (igs *AuthenticatorCertGormStore) LoadOneByName(name string, pl *Preloads) (*AuthenticatorCert, error) {
	out := &AuthenticatorCert{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *AuthenticatorCertGormStore) LoadList(qo *QueryOptions) ([]*AuthenticatorCert, error) {
	out := make([]*AuthenticatorCert, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *AuthenticatorCertGormStore) Create(e *AuthenticatorCert) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *AuthenticatorCertGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &AuthenticatorCert{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *AuthenticatorCertGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*AuthenticatorCert{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *AuthenticatorCertGormStore) DeleteWhereWithTx(tx *gorm.DB, p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(tx, &[]*AuthenticatorCert{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *AuthenticatorCertGormStore) Update(e *AuthenticatorCert) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *AuthenticatorCertGormStore) Patch(e *AuthenticatorCert) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *AuthenticatorCertGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*AuthenticatorCert{})
}
