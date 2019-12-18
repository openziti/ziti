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

const (
	IdentityTypeUserId    = "577104f2-1e3a-4947-a927-7383baefbc9a"
	IdentityTypeServiceId = "c4d66f9d-fe18-4143-85d3-74329c54282b"
	IdentityTypeDeviceId  = "5b53fb49-51b1-4a87-a4e4-edda9716a970"
)

type IdentityTypeStore interface {
	LoadOneById(id string, pl *Preloads) (*IdentityType, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*IdentityType, error)
	LoadOneByName(name string, pl *Preloads) (*IdentityType, error)
	LoadList(qo *QueryOptions) ([]*IdentityType, error)
	Create(e *IdentityType) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *IdentityType) error
	Patch(e *IdentityType) error
	Store
}

type IdentityTypeGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewIdentityTypeGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *IdentityTypeGormStore {
	return &IdentityTypeGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *IdentityTypeGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"name":      "identity_types.name",
		"id":        "identity_types.id",
		"createdAt": "identity_types.created_at",
		"updatedAt": "identity_types.updated_at",
	}
}

func (igs *IdentityTypeGormStore) EntityName() string {
	return "identityType"
}

func (igs *IdentityTypeGormStore) PluralEntityName() string {
	return "identityTypes"
}

func (igs *IdentityTypeGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *IdentityType

	if i, ok = e.(*IdentityType); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *IdentityTypeGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *IdentityTypeGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *IdentityTypeGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *IdentityTypeGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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
func (igs *IdentityTypeGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *IdentityTypeGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *IdentityTypeGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *IdentityTypeGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *IdentityType

	if i, ok = e.(*IdentityType); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *IdentityTypeGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *IdentityType

	if i, ok = e.(*IdentityType); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}
	return igs.Patch(i)
}

func (igs *IdentityTypeGormStore) LoadOneById(id string, pl *Preloads) (*IdentityType, error) {
	out := &IdentityType{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *IdentityTypeGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*IdentityType, error) {
	out := &IdentityType{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *IdentityTypeGormStore) LoadOneByName(name string, pl *Preloads) (*IdentityType, error) {
	out := &IdentityType{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *IdentityTypeGormStore) LoadList(qo *QueryOptions) ([]*IdentityType, error) {
	out := make([]*IdentityType, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *IdentityTypeGormStore) Create(e *IdentityType) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *IdentityTypeGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &IdentityType{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *IdentityTypeGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*IdentityType{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *IdentityTypeGormStore) Update(e *IdentityType) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *IdentityTypeGormStore) Patch(e *IdentityType) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *IdentityTypeGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*IdentityType{})
}
