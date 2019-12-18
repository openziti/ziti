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
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/kataras/go-events"
	"github.com/netfoundry/ziti-edge/controller/predicate"
	"github.com/pkg/errors"
	"gopkg.in/Masterminds/squirrel.v1"
	"reflect"
	"time"
)

const (
	LimitMax      = 500
	OffsetMax     = 100000
	SortMax       = 50
	LimitDefault  = 10
	OffsetDefault = 0
)

func LoadOneById(db *gorm.DB, id string, pl *Preloads, out interface{}) *QueryResult {
	p := &predicate.Predicate{
		Clause: squirrel.Eq{"ID": id},
	}

	return LoadOne(db, p, pl, out)
}

func LoadOne(db *gorm.DB, p *predicate.Predicate, pl *Preloads, out interface{}) *QueryResult {
	q := db
	if p != nil {
		q = p.Apply(q)
	}

	if pl != nil {
		pl.ApplyToQuery(q)
	}

	res := q.Take(out)

	return NewQueryResult(res)
}

func StatList(db *gorm.DB, qo *QueryOptions, typ interface{}) (*ListStats, error) {
	if qo == nil {
		return nil, errors.New("query options can not be nil")
	}

	qo.ValidateAndCorrect()
	q := qo.ApplyPredicateToQuery(db)

	var c int64

	res := q.Find(typ).Count(&c)

	if res.Error != nil {
		return nil, res.Error
	}

	return &ListStats{
		Count:  c,
		Offset: qo.Paging.Offset,
		Limit:  qo.Paging.Limit,
	}, nil

}

func LoadList(db *gorm.DB, qo *QueryOptions, out interface{}) *QueryResult {
	//todo for filters on joined values, one to many relationships will dupe rows, need subqueries that filter to avoid duping or use DISCRETE

	if qo == nil {
		return NewQueryResultF("query options can not be nil")
	}
	t := reflect.TypeOf(out)

	if t.Kind() != reflect.Ptr {
		return NewQueryResultF("out must be a pointer a slice")
	}

	it := reflect.Indirect(reflect.ValueOf(out))

	if it.Kind() != reflect.Slice {
		return NewQueryResultF("out must be a pointer to a slice")
	}

	if reflect.ValueOf(out).IsNil() {
		out = reflect.MakeSlice(reflect.SliceOf(t), 0, 10)
	}

	qo.ValidateAndCorrect()
	q := qo.ApplyToQuery(db)

	res := q.Find(out)

	return NewQueryResult(res)
}

func Create(db *gorm.DB, e BaseDbModel, em events.EventEmmiter) (string, *QueryResult) {
	if e == nil {
		return "", NewQueryResultF("can not create nil entity")
	}

	if e.GetId() == "" {
		id := uuid.New().String()
		e.SetId(id)
	}

	res := db.Create(e)

	qr := NewQueryResult(res)

	if res.RowsAffected > 0 {
		dbWithPreload := db.Set("gorm:auto_preload", true)
		res := LoadOneById(dbWithPreload, e.GetId(), nil, e)

		if res.Error == nil {
			go em.Emit(EventCreate, NewCrudEventDetails(qr, e))
		}
	}

	return e.GetId(), qr
}

func Delete(db *gorm.DB, e BaseDbModel, em events.EventEmmiter) *QueryResult {

	if e == nil {
		return NewQueryResultF("cannot delete nil entity")
	}

	if e.GetId() == "" {
		return NewQueryResultF("can not delete entity with blank id")
	}

	db.Where("id = ?", e.GetId()).Take(e)

	res := db.Delete(e)

	qr := NewQueryResult(res)

	if res.RowsAffected > 0 {
		go em.Emit(EventDelete, NewCrudEventDetails(qr, e))
	}

	return qr
}

func DeleteWhere(db *gorm.DB, slice interface{}, p *predicate.Predicate, em events.EventEmmiter) *QueryResult {
	if p == nil || p.Clause == nil {
		return NewQueryResultF("attempted to delete without predicate")
	}

	clause, args, _ := p.Clause.ToSql()

	et := reflect.TypeOf(slice).Elem()

	e := reflect.Indirect(reflect.New(et)).Interface()

	res := db.Model(e).Where(clause, args...).Find(slice)

	if res.Error != nil || res.RowsAffected == 0 {
		return NewQueryResult(res)
	}

	res = db.Where(clause, args...).Delete(e)

	qr := NewQueryResult(res)

	var entities []BaseDbModel

	vs := reflect.Indirect(reflect.ValueOf(slice))
	for i := 0; i < vs.Len(); i++ {
		v := vs.Index(i).Interface()
		be, ok := v.(BaseDbModel)
		if !ok {
			return NewQueryResultF("could not cast to BaseDbModel")
		}

		entities = append(entities, be)
	}

	if res.RowsAffected > 0 {
		go em.Emit(EventDelete, NewCrudEventDetails(qr, entities...))
	}

	return qr
}

func Update(db *gorm.DB, e BaseDbModel, em events.EventEmmiter) *QueryResult {
	if e == nil || e.GetId() == "" {
		return NewQueryResultF("can not update with blank id")
	}

	now := time.Now()
	e.SetUpdatedAt(&now)

	res := db.Save(e)

	qr := NewQueryResult(res)

	if res.RowsAffected > 0 {
		go em.Emit(EventUpdate, NewCrudEventDetails(qr, e))
	}

	return qr
}

func Patch(db *gorm.DB, e BaseDbModel, em events.EventEmmiter) *QueryResult {
	basePropType := reflect.TypeOf(BaseDbEntity{})

	q := db.Model(e)

	now := time.Now()
	e.SetUpdatedAt(&now)

	eVal := reflect.Indirect(reflect.ValueOf(e))

	fs := []string{"updatedAt"}
	fv := map[string]interface{}{}

	q.Select("updatedAt")

	numF := eVal.NumField()

	//special tag handling
	if tags := e.GetTags(); tags != nil {
		q.Select("tags")
		fs = append(fs, "tags")
		fv["tags"] = tags
	}

	for i := 0; i < numF; i++ {
		fVal := reflect.Indirect(eVal.Field(i))
		fDesc := eVal.Type().Field(i)

		if fDesc.Type != basePropType {
			if fVal.IsValid() {
				q.Select(fDesc.Name)
				fs = append(fs, fDesc.Name)
				fv[fDesc.Name] = fVal.Interface()
			}
		}
	}

	res := q.Updates(fv)

	qr := NewQueryResult(res)

	if res.RowsAffected > 0 {
		ed := NewCrudEventDetails(qr, e)
		ed.FieldsAffected = fs
		go em.Emit(EventPatch, ed)
	}

	return qr
}
