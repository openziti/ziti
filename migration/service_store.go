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

type ServiceStore interface {
	LoadOneById(id string, pl *Preloads) (*Service, error)
	LoadOneByName(name string, pl *Preloads) (*Service, error)
	LoadList(qo *QueryOptions) ([]*Service, error)
	Create(e *Service) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *Service) error
	Patch(e *Service) error
	NewIdentityFilteredStore(i *Identity) ServiceStore
	Store
}

type ServiceGormStore struct {
	db                   *gorm.DB
	dbWithPreloading     *gorm.DB
	filterIdentity       *Identity
	isFilteredByIdentity bool
	events.EventEmmiter
}

func NewServiceGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *ServiceGormStore {
	return &ServiceGormStore{
		db:                   db,
		dbWithPreloading:     dbWithPreloading,
		EventEmmiter:         events.New(),
		isFilteredByIdentity: false,
		filterIdentity:       nil,
	}
}

func (igs *ServiceGormStore) NewIdentityFilteredStore(i *Identity) ServiceStore {
	figs := NewServiceGormStore(igs.db, igs.dbWithPreloading)
	figs.EventEmmiter = igs.EventEmmiter
	figs.isFilteredByIdentity = true
	figs.filterIdentity = i

	return figs
}

func (igs *ServiceGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":           "services.id",
		"name":         "services.name",
		"dns.hostname": "services.dns_hostname",
		"dns.port":     "services.dns_port",
		"createdAt":    "services.created_at",
		"updatedAt":    "services.updated_at",
	}
}

func (igs *ServiceGormStore) EntityName() string {
	return "service"
}

func (igs *ServiceGormStore) PluralEntityName() string {
	return "services"
}

func (igs *ServiceGormStore) BaseCreate(e BaseDbModel) (string, error) {
	i, ok := e.(*Service)
	if !ok {
		return "", fmt.Errorf("invalid type, could not cast to specific type [%s] -> Service", reflect.TypeOf(e).Name())
	}
	return igs.Create(i)
}

func (igs *ServiceGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *ServiceGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *ServiceGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *ServiceGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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
func (igs *ServiceGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *ServiceGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *ServiceGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *ServiceGormStore) BaseUpdate(e BaseDbModel) error {
	i, ok := e.(*Service)
	if !ok {
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> Service", reflect.TypeOf(e).Name())
	}
	return igs.Update(i)
}

func (igs *ServiceGormStore) BasePatch(e BaseDbModel) error {
	i, ok := e.(*Service)
	if !ok {
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> Service", reflect.TypeOf(e).Name())
	}
	return igs.Patch(i)
}

func (igs *ServiceGormStore) LoadOneById(id string, pl *Preloads) (*Service, error) {
	p := &predicate.Predicate{
		Clause: squirrel.Eq{"services.ID": id},
	}
	return igs.LoadOne(p, pl)

}

func (igs *ServiceGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*Service, error) {
	out := &Service{}

	rp := p

	if igs.isFilteredByIdentity {
		rp = &predicate.Predicate{
			Callback: func(q *gorm.DB) *gorm.DB {
				q = q.
					Table("services").
					Select("services.*").
					Joins("inner join app_wan_services on services.id = app_wan_services.service_id").
					Joins("inner join app_wan_identities on app_wan_services.app_wan_id = app_wan_identities.app_wan_id and app_wan_identities.identity_id = ?", igs.filterIdentity.ID)

				if p != nil {
					q = p.Apply(q)
				}

				return q
			},
		}
	}

	res := LoadOne(igs.dbWithPreloading, rp, pl, out)
	return out, res.Error
}

func (igs *ServiceGormStore) LoadOneByName(name string, pl *Preloads) (*Service, error) {
	out := &Service{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *ServiceGormStore) LoadList(qo *QueryOptions) ([]*Service, error) {
	out := make([]*Service, qo.Paging.Limit)

	if igs.isFilteredByIdentity {
		op := qo.Predicate
		qo.Predicate = &predicate.Predicate{
			Callback: func(q *gorm.DB) *gorm.DB {
				q = q.
					Table("services").
					Select("services.*").
					Joins("inner join app_wan_services on services.id = app_wan_services.service_id").
					Joins("inner join app_wan_identities on app_wan_services.app_wan_id = app_wan_identities.app_wan_id and app_wan_identities.identity_id = ?", igs.filterIdentity.ID)

				if op != nil {
					q = op.Apply(q)
				}

				return q
			},
		}
	}

	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *ServiceGormStore) Create(e *Service) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *ServiceGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &Service{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *ServiceGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*Service{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *ServiceGormStore) Update(e *Service) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *ServiceGormStore) Patch(e *Service) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *ServiceGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*Service{})
}
