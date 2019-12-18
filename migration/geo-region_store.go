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

type GeoRegionStore interface {
	LoadOneById(id string, pl *Preloads) (*GeoRegion, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*GeoRegion, error)
	LoadOneByName(name string, pl *Preloads) (*GeoRegion, error)
	LoadList(qo *QueryOptions) ([]*GeoRegion, error)
	Create(e *GeoRegion) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *GeoRegion) error
	Patch(e *GeoRegion) error
	Store
}

type GeoRegionGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewGeoRegionGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *GeoRegionGormStore {
	return &GeoRegionGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *GeoRegionGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"name":      "geo_regions.name",
		"id":        "geo_regions.id",
		"createdAt": "geo_regions.created_at",
		"updatedAt": "geo_regions.updated_at",
	}
}

func (igs *GeoRegionGormStore) EntityName() string {
	return "geoRegion"
}

func (igs *GeoRegionGormStore) PluralEntityName() string {
	return "geoRegions"
}

func (igs *GeoRegionGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *GeoRegion

	if i, ok = e.(*GeoRegion); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *GeoRegionGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *GeoRegionGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *GeoRegionGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *GeoRegionGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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
func (igs *GeoRegionGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *GeoRegionGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *GeoRegionGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *GeoRegionGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *GeoRegion

	if i, ok = e.(*GeoRegion); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *GeoRegionGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *GeoRegion

	if i, ok = e.(*GeoRegion); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}
	return igs.Patch(i)
}

func (igs *GeoRegionGormStore) LoadOneById(id string, pl *Preloads) (*GeoRegion, error) {
	out := &GeoRegion{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *GeoRegionGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*GeoRegion, error) {
	out := &GeoRegion{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *GeoRegionGormStore) LoadOneByName(name string, pl *Preloads) (*GeoRegion, error) {
	out := &GeoRegion{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *GeoRegionGormStore) LoadList(qo *QueryOptions) ([]*GeoRegion, error) {
	out := make([]*GeoRegion, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *GeoRegionGormStore) Create(e *GeoRegion) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *GeoRegionGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &GeoRegion{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *GeoRegionGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*GeoRegion{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *GeoRegionGormStore) Update(e *GeoRegion) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *GeoRegionGormStore) Patch(e *GeoRegion) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *GeoRegionGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*GeoRegion{})
}
