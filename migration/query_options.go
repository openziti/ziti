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
	"github.com/jinzhu/gorm"
	"github.com/netfoundry/ziti-edge/controller/predicate"
)

type QueryOptions struct {
	Predicate *predicate.Predicate
	Sort      []predicate.SortField
	Paging    *predicate.Paging
}

func (qo *QueryOptions) ValidateAndCorrect() {
	if qo.Sort == nil {
		qo.Sort = []predicate.SortField{}
	} else if len(qo.Sort) > SortMax {
		qo.Sort = qo.Sort[:SortMax]
	}

	if qo.Paging == nil {
		qo.Paging = &predicate.Paging{
			Limit:  LimitDefault,
			Offset: OffsetDefault,
		}
	} else {
		if qo.Paging.Limit > LimitMax {
			qo.Paging.Limit = LimitMax
		}

		if qo.Paging.Limit <= 0 {
			qo.Paging.Limit = LimitDefault
		}

		if qo.Paging.Offset > OffsetMax {
			qo.Paging.Offset = OffsetMax
		}

		if qo.Paging.Offset <= 0 {
			qo.Paging.Offset = OffsetDefault
		}
	}
}

func (qo *QueryOptions) ApplyToQuery(q *gorm.DB) *gorm.DB {
	if qo == nil {
		return q
	}

	q = qo.ApplyPredicateToQuery(q)

	if qo.Sort != nil {
		for _, s := range qo.Sort {
			q = q.Order(s.String())
		}
	}

	if qo.Paging != nil && !qo.Paging.ReturnAll {
		q = q.Limit(qo.Paging.Limit)
		q = q.Offset(qo.Paging.Offset)
	}

	return q
}

func (qo *QueryOptions) ApplyPredicateToQuery(q *gorm.DB) *gorm.DB {
	if qo == nil {
		return q
	}

	if qo.Predicate != nil {
		q = qo.Predicate.Apply(q)
	}

	return q
}
