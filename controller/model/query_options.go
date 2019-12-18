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

package model

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/predicate"
	"strconv"
	"strings"
)

const (
	LimitMax      = 500
	OffsetMax     = 100000
	LimitDefault  = 10
	OffsetDefault = 0
)

func NewQueryOptions(predicate string, paging *predicate.Paging, sort string) *QueryOptions {
	return &QueryOptions{
		Predicate: predicate,
		Paging:    paging,
		Sort:      sort,
	}
}

type QueryOptions struct {
	Predicate  string
	Sort       string
	Paging     *predicate.Paging
	finalQuery string
}

func (qo *QueryOptions) String() string {
	if qo == nil {
		return "nil"
	}
	return fmt.Sprintf("[QueryOption Predicate: '%v', Sort: '%v', Paging: '%v']", qo.Predicate, qo.Sort, qo.Paging)
}

func (qo *QueryOptions) ValidateAndCorrect() {
	// Sort limit is handled in ScanEntityImpl.NewScanner
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

func (qo *QueryOptions) getOriginalFullQuery() string {
	return qo.getFullQuery(qo.Predicate)
}

func (qo *QueryOptions) getFinalFullQuery() string {
	if qo.finalQuery != "" {
		return qo.getFullQuery(qo.finalQuery)
	}
	return qo.getFullQuery(qo.Predicate)
}

func (qo *QueryOptions) getFullQuery(predicate string) string {
	qo.ValidateAndCorrect()
	pfxlog.Logger().Debugf("query: %v", qo)
	queryBuilder := strings.Builder{}
	if qo.Predicate == "" {
		queryBuilder.WriteString("true")
	} else {
		queryBuilder.WriteString(predicate)
	}

	if qo.Sort != "" {
		queryBuilder.WriteString(" sort by ")
		queryBuilder.WriteString(qo.Sort)
	}

	if qo.Paging != nil {
		queryBuilder.WriteString(" skip ")
		queryBuilder.WriteString(strconv.FormatInt(qo.Paging.Offset, 10))
		queryBuilder.WriteString(" limit ")
		if qo.Paging.ReturnAll {
			queryBuilder.WriteString("none")
		} else {
			queryBuilder.WriteString(strconv.FormatInt(qo.Paging.Limit, 10))
		}
	}
	return queryBuilder.String()
}