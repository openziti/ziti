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

type NetworkSessionCertStore interface {
	LoadOneById(id string, pl *Preloads) (*NetworkSessionCert, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*NetworkSessionCert, error)
	LoadOneByName(name string, pl *Preloads) (*NetworkSessionCert, error)
	LoadList(qo *QueryOptions) ([]*NetworkSession, error)
	Create(e *NetworkSession) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *NetworkSession) error
	Patch(e *NetworkSession) error
	NewIdentityFilteredStore(i *Identity) NetworkSessionCertStore
	Store
}

type NetworkSessionCertGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewNetworkSessionCertGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *NetworkSessionCertGormStore {
	ns := &NetworkSessionCertGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}

	return ns
}

func (igs *NetworkSessionCertGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":        "network_sessions.id",
		"sessionId": "network_sessions.session_id",
		"serviceId": "network_sessions.service_id",
		"createdAt": "network_sessions.created_at",
		"updatedAt": "network_sessions.updated_at",
	}
}

func (igs *NetworkSessionCertGormStore) EntityName() string {
	return "networkSession"
}

func (igs *NetworkSessionCertGormStore) PluralEntityName() string {
	return "networkSessions"
}

func (igs *NetworkSessionCertGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *NetworkSession

	if i, ok = e.(*NetworkSession); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type  [%s] -> [%s]", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *NetworkSessionCertGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *NetworkSessionCertGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *NetworkSessionCertGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *NetworkSessionCertGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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
func (igs *NetworkSessionCertGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *NetworkSessionCertGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *NetworkSessionCertGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *NetworkSessionCertGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *NetworkSession

	if i, ok = e.(*NetworkSession); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type [%s] -> [%s]", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *NetworkSessionCertGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *NetworkSessionCert

	if i, ok = e.(*NetworkSessionCert); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *NetworkSessionCertGormStore) LoadOneById(id string, pl *Preloads) (*NetworkSessionCert, error) {
	p := &predicate.Predicate{
		Clause: squirrel.Eq{"network_sessions.ID": id},
	}
	return igs.LoadOne(p, pl)
}

func (igs *NetworkSessionCertGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*NetworkSessionCert, error) {
	out := &NetworkSessionCert{}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *NetworkSessionCertGormStore) LoadOneByName(name string, pl *Preloads) (*NetworkSessionCert, error) {
	out := &NetworkSessionCert{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *NetworkSessionCertGormStore) LoadList(qo *QueryOptions) ([]*NetworkSessionCert, error) {
	out := make([]*NetworkSessionCert, qo.Paging.Limit)

	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *NetworkSessionCertGormStore) Create(e *NetworkSession) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *NetworkSessionCertGormStore) DeleteWithTx(e *NetworkSession, tx *gorm.DB) error {
	res := Delete(tx, e, igs)

	return res.Error
}

func (igs *NetworkSessionCertGormStore) DeleteById(id string) error {
	res := Delete(igs.dbWithPreloading, &NetworkSession{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *NetworkSessionCertGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.dbWithPreloading, &[]*NetworkSession{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *NetworkSessionCertGormStore) Update(e *NetworkSession) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *NetworkSessionCertGormStore) Patch(e *NetworkSessionCert) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *NetworkSessionCertGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*NetworkSession{})
}
