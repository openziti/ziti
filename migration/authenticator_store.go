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

type AuthenticatorStore interface {
	LoadOneById(id string, pl *Preloads) (*Authenticator, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*Authenticator, error)
	LoadOneByName(name string, pl *Preloads) (*Authenticator, error)
	LoadList(qo *QueryOptions) ([]*Authenticator, error)
	Create(e *Authenticator) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *Authenticator) error
	Patch(e *Authenticator) error
	Store
}

type AuthenticatorGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewAuthenticatorGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *AuthenticatorGormStore {
	return &AuthenticatorGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *AuthenticatorGormStore) EntityName() string {
	return "authenticator"
}

func (igs *AuthenticatorGormStore) PluralEntityName() string {
	return "authenticators"
}

func (igs *AuthenticatorGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":        "clusters.id",
		"name":      "clusters.name",
		"createdAt": "clusters.created_at",
		"updatedAt": "clusters.updated_at",
	}
}

func (igs *AuthenticatorGormStore) BaseCreate(e BaseDbModel) (string, error) {
	i, ok := e.(*Authenticator)
	if !ok {
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> Authenticator", reflect.TypeOf(e).Name())
	}
	return igs.Create(i)
}

func (igs *AuthenticatorGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *AuthenticatorGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *AuthenticatorGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *AuthenticatorGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *AuthenticatorGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *AuthenticatorGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *AuthenticatorGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *AuthenticatorGormStore) BaseUpdate(e BaseDbModel) error {
	i, ok := e.(*Authenticator)
	if !ok {
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> Authenticator", reflect.TypeOf(e).Name())
	}
	return igs.Update(i)
}

func (igs *AuthenticatorGormStore) BasePatch(e BaseDbModel) error {
	i, ok := e.(*Authenticator)
	if !ok {
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> Authenticator", reflect.TypeOf(e).Name())
	}
	return igs.Patch(i)
}

func (igs *AuthenticatorGormStore) LoadOneById(id string, pl *Preloads) (*Authenticator, error) {
	out := &Authenticator{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *AuthenticatorGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*Authenticator, error) {
	out := &Authenticator{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *AuthenticatorGormStore) LoadOneByName(name string, pl *Preloads) (*Authenticator, error) {
	out := &Authenticator{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *AuthenticatorGormStore) LoadList(qo *QueryOptions) ([]*Authenticator, error) {
	out := make([]*Authenticator, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *AuthenticatorGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*Authenticator{})
}

func (igs *AuthenticatorGormStore) Create(e *Authenticator) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *AuthenticatorGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &Authenticator{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *AuthenticatorGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*Authenticator{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *AuthenticatorGormStore) Update(e *Authenticator) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *AuthenticatorGormStore) Patch(e *Authenticator) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}
