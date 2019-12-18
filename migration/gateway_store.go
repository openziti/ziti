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

type GatewayStore interface {
	LoadOneById(id string, pl *Preloads) (*Gateway, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*Gateway, error)
	LoadOneByName(name string, pl *Preloads) (*Gateway, error)
	LoadList(qo *QueryOptions) ([]*Gateway, error)
	Create(e *Gateway) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *Gateway) error
	Patch(e *Gateway) error
	Store
}

type GatewayGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewGatewayGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *GatewayGormStore {
	return &GatewayGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *GatewayGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":                  "gateways.id",
		"name":                "gateways.name",
		"fingerprint":         "gateways.fingerprint",
		"clusterId":           "gateways.cluster_id",
		"isVerified":          "gateways.is_verified",
		"isOnline":            "gateways.is_online",
		"enrollmentToken":     "gateways.enrollment_token",
		"enrollmentCreatedAt": "gateways.enrollment_created_at",
		"enrollmentExpiresAt": "gateways.enrollment_expires_at",
		"createdAt":           "gateways.created_at",
		"updatedAt":           "gateways.updated_at",
	}
}

func (igs *GatewayGormStore) EntityName() string {
	return "gateway"
}

func (igs *GatewayGormStore) PluralEntityName() string {
	return "gateways"
}

func (igs *GatewayGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *Gateway

	if i, ok = e.(*Gateway); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *GatewayGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *GatewayGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *GatewayGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *GatewayGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *GatewayGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *GatewayGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*Gateway{})
}

func (igs *GatewayGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *GatewayGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *GatewayGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *Gateway

	if i, ok = e.(*Gateway); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *GatewayGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *Gateway

	if i, ok = e.(*Gateway); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *GatewayGormStore) LoadOneById(id string, pl *Preloads) (*Gateway, error) {
	out := &Gateway{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *GatewayGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*Gateway, error) {
	out := &Gateway{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *GatewayGormStore) LoadOneByName(name string, pl *Preloads) (*Gateway, error) {
	out := &Gateway{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *GatewayGormStore) LoadList(qo *QueryOptions) ([]*Gateway, error) {
	out := make([]*Gateway, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *GatewayGormStore) Create(e *Gateway) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *GatewayGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &Gateway{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *GatewayGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*Gateway{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *GatewayGormStore) Update(e *Gateway) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *GatewayGormStore) Patch(e *Gateway) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}
