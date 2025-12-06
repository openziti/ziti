/*
	Copyright NetFoundry Inc.

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

package routes

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-openapi/strfmt"
	edgeRestModel "github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/controller/models"
	fabricRestModel "github.com/openziti/ziti/controller/rest_model"
)

func GetModelQueryOptionsFromRequest(r *http.Request) (*PublicQueryOptions, error) {
	filter := r.URL.Query().Get("filter")
	sort := r.URL.Query().Get("sort")

	pg, err := GetRequestPaging(r)

	if err != nil {
		return nil, err
	}

	return &PublicQueryOptions{
		Predicate: filter,
		Sort:      sort,
		Paging:    pg,
	}, nil
}

func GetRequestPaging(r *http.Request) (*Paging, error) {
	l := r.URL.Query().Get("limit")
	o := r.URL.Query().Get("offset")

	var p *Paging

	if l != "" {
		i, err := strconv.ParseInt(l, 10, 64)

		if err != nil {
			return nil, errorz.NewInvalidPagination(errorz.NewFieldError("could not parse limit, value is not an integer", "limit", l))
		}
		p = &Paging{}
		p.Limit = i
	}

	if o != "" {
		i, err := strconv.ParseInt(o, 10, 64)

		if err != nil {
			return nil, errorz.NewInvalidPagination(errorz.NewFieldError("could not parse offset, value is not an integer", "offset", o))
		}
		if p == nil {
			p = &Paging{}
		}
		p.Offset = i
	}

	return p, nil
}

type QueryResult struct {
	Result           interface{}
	Count            int64
	Limit            int64
	Offset           int64
	FilterableFields []string
}

func NewQueryResult(result interface{}, metadata *models.QueryMetaData) *QueryResult {
	return &QueryResult{
		Result:           result,
		Count:            metadata.Count,
		Limit:            metadata.Limit,
		Offset:           metadata.Offset,
		FilterableFields: metadata.FilterableFields,
	}
}

func TagsOrDefault(tags *edgeRestModel.Tags) map[string]interface{} {
	if tags == nil || tags.SubTags == nil {
		return map[string]interface{}{}
	}
	return tags.SubTags
}

func AttributesOrDefault(attributes *edgeRestModel.Attributes) edgeRestModel.Attributes {
	if attributes == nil {
		return edgeRestModel.Attributes{}
	}

	return *attributes
}

func ValueOrDefault[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

func BoolOrDefault(val *bool) bool {
	if val == nil {
		return false
	}

	return *val
}

func Int64OrDefault(val *int64) int64 {
	if val == nil {
		return 0
	}

	return *val
}

func DateTimePtrOrNil(time *time.Time) *strfmt.DateTime {
	if time == nil {
		return nil
	}
	dateTime := strfmt.DateTime(*time)
	return &dateTime
}

func FabricTagsOrDefault(tags *fabricRestModel.Tags) map[string]interface{} {
	if tags == nil || tags.SubTags == nil {
		return map[string]interface{}{}
	}
	return tags.SubTags
}
