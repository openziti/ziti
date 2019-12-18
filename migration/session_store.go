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
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/predicate"
	"gopkg.in/Masterminds/squirrel.v1"
	"reflect"
	"time"
)

type SessionStore interface {
	LoadOneById(id string, pl *Preloads) (*Session, error)
	LoadOne(p *predicate.Predicate, pl *Preloads) (*Session, error)
	LoadOneByName(name string, pl *Preloads) (*Session, error)
	LoadList(qo *QueryOptions) ([]*Session, error)
	Create(e *Session) (string, error)
	DeleteById(id string) error
	DeleteWhere(p *predicate.Predicate) (int64, error)
	IdentifierMap() *predicate.IdentifierMap
	Update(e *Session) error
	Patch(e *Session) error
	MarkActivity(strings []string) error
	Store
	DeleteIfNotUpdatedBy(updatedBy time.Time) error
}

type SessionGormStore struct {
	db               *gorm.DB
	dbWithPreloading *gorm.DB
	events.EventEmmiter
}

func NewSessionGormStore(db *gorm.DB, dbWithPreloading *gorm.DB) *SessionGormStore {
	return &SessionGormStore{
		db:               db,
		dbWithPreloading: dbWithPreloading,
		EventEmmiter:     events.New(),
	}
}

func (igs *SessionGormStore) IdentifierMap() *predicate.IdentifierMap {
	return &predicate.IdentifierMap{
		"id":         "sessions.id",
		"identityId": "sessions.identity_id",
		"createdAt":  "sessions.created_at",
		"updatedAt":  "sessions.updated_at",
	}
}

func (igs *SessionGormStore) EntityName() string {
	return "session"
}

func (igs *SessionGormStore) PluralEntityName() string {
	return "sessions"
}

func (igs *SessionGormStore) BaseCreate(e BaseDbModel) (string, error) {
	ok := false
	var i *Session

	if i, ok = e.(*Session); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return "", fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Create(i)
}

func (igs *SessionGormStore) BaseDeleteById(id string) error {
	return igs.DeleteById(id)
}

func (igs *SessionGormStore) BaseDeleteWhere(p *predicate.Predicate) (int64, error) {
	return igs.DeleteWhere(p)
}

func (igs *SessionGormStore) BaseIdentifierMap() *predicate.IdentifierMap {
	return igs.IdentifierMap()
}

func (igs *SessionGormStore) BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error) {
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
func (igs *SessionGormStore) BaseStatsList(qo *QueryOptions) (*ListStats, error) {
	return igs.StatsList(qo)

}

func (igs *SessionGormStore) BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOne(p, pl)
}

func (igs *SessionGormStore) BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error) {
	return igs.LoadOneById(id, pl)
}

func (igs *SessionGormStore) BaseUpdate(e BaseDbModel) error {
	ok := false
	var i *Session

	if i, ok = e.(*Session); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Update(i)
}

func (igs *SessionGormStore) BasePatch(e BaseDbModel) error {
	ok := false
	var i *Session

	if i, ok = e.(*Session); !ok {
		et := reflect.TypeOf(e)
		it := reflect.TypeOf(i)
		return fmt.Errorf("invalid type, could not cast to specific type: %s -> %s", et.Name(), it.Name())
	}

	return igs.Patch(i)
}

func (igs *SessionGormStore) LoadOneById(id string, pl *Preloads) (*Session, error) {
	out := &Session{}
	res := LoadOneById(igs.dbWithPreloading, id, pl, out)
	return out, res.Error
}

func (igs *SessionGormStore) LoadOne(p *predicate.Predicate, pl *Preloads) (*Session, error) {
	out := &Session{}
	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *SessionGormStore) LoadOneByName(name string, pl *Preloads) (*Session, error) {
	out := &Session{}

	p := &predicate.Predicate{
		Clause: squirrel.Eq{"name": name},
	}

	res := LoadOne(igs.dbWithPreloading, p, pl, out)
	return out, res.Error
}

func (igs *SessionGormStore) LoadList(qo *QueryOptions) ([]*Session, error) {
	out := make([]*Session, qo.Paging.Limit)
	res := LoadList(igs.dbWithPreloading, qo, &out)
	return out, res.Error
}

func (igs *SessionGormStore) Create(e *Session) (string, error) {
	id, res := Create(igs.db, e, igs)

	return id, res.Error
}

func (igs *SessionGormStore) DeleteById(id string) error {
	res := Delete(igs.db, &Session{BaseDbEntity: BaseDbEntity{ID: id}}, igs)

	return res.Error
}

func (igs *SessionGormStore) DeleteWhere(p *predicate.Predicate) (int64, error) {
	res := DeleteWhere(igs.db, &[]*Session{}, p, igs)

	return res.RowsAffected, res.Error
}

func (igs *SessionGormStore) Update(e *Session) error {
	res := Update(igs.db, e, igs)

	return res.Error
}

func (igs *SessionGormStore) Patch(e *Session) error {
	res := Patch(igs.db, e, igs)

	return res.Error
}

func (igs *SessionGormStore) StatsList(qo *QueryOptions) (*ListStats, error) {
	return StatList(igs.db, qo, &[]*Session{})
}

func (igs *SessionGormStore) MarkActivity(tokens []string) error {
	res := igs.db.Table("sessions").Where("token IN (?)", tokens).Updates(map[string]interface{}{"updated_at": time.Now()})

	return res.Error
}

func (igs *SessionGormStore) DeleteIfNotUpdatedBy(updatedBy time.Time) error {
	var sessionIds []string

	res := igs.db.Table("sessions").
		Select("sessions.id").
		Where("updated_at < ?", updatedBy).
		Pluck("sessions.id", &sessionIds)

	if res.Error != nil {
		return res.Error
	}

	log := pfxlog.Logger()
	for _, sessionId := range sessionIds {
		log.WithField("sessionId", sessionId).
			Info("session timed out, removing")
		err := igs.DeleteById(sessionId)
		if err != nil && !IsErrNotFoundErr(err) {
			return err
		}
	}

	return nil
}
