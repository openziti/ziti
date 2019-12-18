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
	"reflect"
)

const IdentityDefaultAdminId = "da71c941-576b-4b2a-9af2-53867c6d1ec5"

type IdentityStore interface {
	LoadOneById(id string, pl *Preloads) (*Identity, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*Identity, error)
	LoadList(qo *QueryOptions) ([]*Identity, error)
	Create(e *Identity) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *Identity) error
	Patch(e *Identity) error
	Store
}

type IdentityGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewIdentityGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *IdentityGormStore {
	return &IdentityGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *IdentityGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"name":           "identities.name",
		"identityTypeId": "identities.identityTypeId",
		"isDefaultAdmin": "identities.is_default_admin",
		"isAdmin":        "identities.is_admin",
		"id":             "identities.id",
		"typeId":         "identities.identity_type_id",
		"createdAt":      "identities.created_at",
		"updatedAt":      "identities.updated_at",
	}
}

func (igs *IdentityGormStore) EntityName() string {
	return "identity"
}

func (igs *IdentityGormStore) PluralEntityName() string {
	return "identities"
}

func (igs *IdentityGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *Identity

	if i, ok = e.(*Identity); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}
	return igs.Create(i)
}

func (igs *IdentityGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *IdentityGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *IdentityGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *IdentityGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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
func (igs *IdentityGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *IdentityGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *IdentityGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *IdentityGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *Identity

	if i, ok = e.(*Identity); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *IdentityGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *Identity

	if i, ok = e.(*Identity); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *IdentityGormStore) LoadOneById(id string, pl *Preloads) (*Identity, error) {
	out := &Identity{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *IdentityGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*Identity, error) {
	out := &Identity{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *IdentityGormStore) LoadList(qo *QueryOptions) ([]*Identity, error) {
	out := make([]*Identity, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *IdentityGormStore) Create(e *Identity) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *IdentityGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &Identity{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *IdentityGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*Identity{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *IdentityGormStore) Update(e *Identity) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *IdentityGormStore) Patch(e *Identity) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *IdentityGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*Identity{})
}
