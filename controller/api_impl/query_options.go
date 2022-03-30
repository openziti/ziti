/*
	Copyright NetFoundry, Inc.

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

package api_impl

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
)

const (
	LimitMax = 500
)

// PublicQueryOptions represents external query options, which limits some query options (i.e. return all vs max limit). PublicQueryOptions
// can be used internally as long as the public option limitations are fitting.
type PublicQueryOptions struct {
	Predicate string
	Sort      string
	Paging    *Paging
}

func (qo *PublicQueryOptions) String() string {
	if qo == nil {
		return "nil"
	}
	return fmt.Sprintf("[QueryOption Predicate: '%v', Sort: '%v', Paging: '%v']", qo.Predicate, qo.Sort, qo.Paging)
}

func (qo *PublicQueryOptions) getFullQuery(store boltz.ListStore) (ast.Query, error) {
	if qo.Predicate == "" {
		qo.Predicate = "true"
	}

	query, err := ast.Parse(store, qo.Predicate)
	if err != nil {
		return nil, errorz.NewInvalidFilter(err)
	}

	if err = boltz.ValidateSymbolsArePublic(query, store); err != nil {
		return nil, errorz.NewInvalidFilter(err)
	}

	pfxlog.Logger().Debugf("query: %v", qo)

	if qo.Paging != nil {
		if query.GetLimit() == nil {
			if qo.Paging.ReturnAll {
				//public external queries cannot be "return all"
				query.SetLimit(LimitMax)
			} else {
				query.SetLimit(qo.Paging.Limit)
			}
		}
		if query.GetSkip() == nil {
			query.SetSkip(qo.Paging.Offset)
		}
	}

	// sort out sorts
	sortFields := query.GetSortFields()
	if len(sortFields) == 0 && qo.Sort != "" {
		sortQueryString := "true sort by " + qo.Sort

		sortQuery, err := ast.Parse(store, sortQueryString)
		if err != nil {
			return nil, errorz.NewInvalidSort(err)
		}

		if err = boltz.ValidateSymbolsArePublic(sortQuery, store); err != nil {
			return nil, errorz.NewInvalidSort(err)
		}

		if err = query.AdoptSortFields(sortQuery); err != nil {
			return nil, errorz.NewInvalidSort(err)
		}
	}

	return query, nil
}

type Paging struct {
	Offset    int64
	Limit     int64
	ReturnAll bool
}

func (paging *Paging) String() string {
	if paging == nil {
		return "nil"
	}
	return fmt.Sprintf("[Paging Offset: '%v', Limit: '%v', ReturnAll: '%v']", paging.Offset, paging.Limit, paging.ReturnAll)
}
