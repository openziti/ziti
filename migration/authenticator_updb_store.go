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

type AuthenticatorUpdbStore interface {
	LoadOneById(id string, pl *Preloads) (*AuthenticatorUpdb, error)
	BaseLoadOneByAuthenticatorId(authId string, pl *Preloads) (BaseDbModel, error)
	LoadOneByAuthenticatorId(authId string, pl *Preloads) (*AuthenticatorUpdb, error)
	LoadOneByIdentityId(identId string, pl *Preloads) (*AuthenticatorUpdb, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*AuthenticatorUpdb, error)
	LoadOneByName(name string, pl *Preloads) (*AuthenticatorUpdb, error)
	LoadList(qo *QueryOptions) ([]*AuthenticatorUpdb, error)
	Create(e *AuthenticatorUpdb) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *AuthenticatorUpdb) error
	Patch(e *AuthenticatorUpdb) error
	Store
}

type AuthenticatorUpdbGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewAuthenticatorUpdbGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *AuthenticatorUpdbGormStore {
	return &AuthenticatorUpdbGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *AuthenticatorUpdbGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id": "authenticator_updbs.id",
	}
}

func (igs *AuthenticatorUpdbGormStore) EntityName() string {
	return "authenticatorUpdb"
}

func (igs *AuthenticatorUpdbGormStore) PluralEntityName() string {
	return "authenticatorUpdbs"
}

func (igs *AuthenticatorUpdbGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *AuthenticatorUpdb

	if i, ok = e.(*AuthenticatorUpdb); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *AuthenticatorUpdbGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *AuthenticatorUpdbGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *AuthenticatorUpdbGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *AuthenticatorUpdbGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *AuthenticatorUpdbGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)
}

func (igs *AuthenticatorUpdbGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *AuthenticatorUpdbGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *AuthenticatorUpdbGormStore) BaseLoadOneByAuthenticatorId(authId string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneByAuthenticatorId(authId, pl)
}

func (igs *AuthenticatorUpdbGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *AuthenticatorUpdb

	if i, ok = e.(*AuthenticatorUpdb); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *AuthenticatorUpdbGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *AuthenticatorUpdb

	if i, ok = e.(*AuthenticatorUpdb); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *AuthenticatorUpdbGormStore) LoadOneByAuthenticatorId(authId string, pl *Preloads) (*AuthenticatorUpdb, error) {
	out := &AuthenticatorUpdb{}
	p := &predicate.Predicate{
		Clause: squirrel.Eq{"authenticator_id": authId},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)

	return out, res.Error
}

func (igs *AuthenticatorUpdbGormStore) LoadOneByIdentityId(identId string, pl *Preloads) (*AuthenticatorUpdb, error) {
	ret, err := igs.LoadOne(&predicate.Predicate{
		Clause: predicate.InSubSelect{Column: "authenticator_id", Select: squirrel.Select("id").
			From("authenticators a").
			Where("a.identity_id = ? and authenticator_updbs.authenticator_id = a.id", identId)},
	}, nil)

	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (igs *AuthenticatorUpdbGormStore) LoadOneById(id string, pl *Preloads) (*AuthenticatorUpdb, error) {
	out := &AuthenticatorUpdb{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *AuthenticatorUpdbGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*AuthenticatorUpdb, error) {
	out := &AuthenticatorUpdb{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *AuthenticatorUpdbGormStore) LoadOneByName(name string, pl *Preloads) (*AuthenticatorUpdb, error) {
	out := &AuthenticatorUpdb{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"username": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *AuthenticatorUpdbGormStore) LoadList(qo *QueryOptions) ([]*AuthenticatorUpdb, error) {
	out := make([]*AuthenticatorUpdb, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *AuthenticatorUpdbGormStore) Create(e *AuthenticatorUpdb) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *AuthenticatorUpdbGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &AuthenticatorUpdb{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *AuthenticatorUpdbGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*AuthenticatorUpdb{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *AuthenticatorUpdbGormStore) Update(e *AuthenticatorUpdb) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *AuthenticatorUpdbGormStore) Patch(e *AuthenticatorUpdb) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *AuthenticatorUpdbGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*AuthenticatorUpdb{})
}
