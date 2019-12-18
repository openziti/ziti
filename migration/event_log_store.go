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

type EventLogStore interface {
	LoadOneById(id string, pl *Preloads) (*EventLog, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*EventLog, error)
	LoadOneByName(name string, pl *Preloads) (*EventLog, error)
	LoadOneByToken(token string, pl *Preloads) (*EventLog, error)
	LoadList(qo *QueryOptions) ([]*EventLog, error)
	Create(e *EventLog) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *EventLog) error
	Patch(e *EventLog) error
	Store
}

type EventLogGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewEventLogGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *EventLogGormStore {
	return &EventLogGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *EventLogGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":               "event_logs.id",
		"expiresAt":        "event_logs.expires_at",
		"createdAt":        "event_logs.created_at",
		"updatedAt":        "event_logs.updated_at",
		"type":             "event_logs.type",
		"actorType":        "event_logs.actor_type",
		"actorId":          "event_logs.actor_id",
		"entityType":       "event_logs.entity_type",
		"entityId":         "event_logs.entity_id",
		"formattedMessage": "event_logs.entity_id",
		"formatString":     "event_logs.format_string",
	}
}

func (igs *EventLogGormStore) EntityName() string {
	return "event_log"
}

func (igs *EventLogGormStore) PluralEntityName() string {
	return "event_logs"
}

func (igs *EventLogGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *EventLog

	if i, ok = e.(*EventLog); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *EventLogGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *EventLogGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *EventLogGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *EventLogGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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

func (igs *EventLogGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *EventLogGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *EventLogGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *EventLogGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *EventLog

	if i, ok = e.(*EventLog); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *EventLogGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *EventLog

	if i, ok = e.(*EventLog); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *EventLogGormStore) LoadOneById(id string, pl *Preloads) (*EventLog, error) {
	out := &EventLog{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *EventLogGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*EventLog, error) {
	out := &EventLog{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *EventLogGormStore) LoadOneByName(name string, pl *Preloads) (*EventLog, error) {
	out := &EventLog{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *EventLogGormStore) LoadOneByToken(token string, pl *Preloads) (*EventLog, error) {
	out := &EventLog{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"token": token},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *EventLogGormStore) LoadList(qo *QueryOptions) ([]*EventLog, error) {
	out := make([]*EventLog, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *EventLogGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*EventLog{})
}

func (igs *EventLogGormStore) Create(e *EventLog) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *EventLogGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &EventLog{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *EventLogGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*EventLog{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *EventLogGormStore) Update(e *EventLog) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *EventLogGormStore) Patch(e *EventLog) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}
