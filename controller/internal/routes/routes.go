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

package routes

import (
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/predicate"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/migration"
	"github.com/openziti/fabric/controller/models"
	"net/http"
	"strconv"
)

type ToBaseModelConverter interface {
	ToBaseModel() migration.BaseDbModel
}

type CrudRouter interface {
	List(ae *env.AppEnv, rc *response.RequestContext)
	Detail(ae *env.AppEnv, rc *response.RequestContext)
	Create(ae *env.AppEnv, rc *response.RequestContext)
	Delete(ae *env.AppEnv, rc *response.RequestContext)
	Update(ae *env.AppEnv, rc *response.RequestContext)
	Patch(ae *env.AppEnv, rc *response.RequestContext)
}

type CreateReadDeleteOnlyRouter interface {
	List(ae *env.AppEnv, rc *response.RequestContext)
	Detail(ae *env.AppEnv, rc *response.RequestContext)
	Create(ae *env.AppEnv, rc *response.RequestContext)
	Delete(ae *env.AppEnv, rc *response.RequestContext)
}

type ReadDeleteRouter interface {
	List(ae *env.AppEnv, rc *response.RequestContext)
	Detail(ae *env.AppEnv, rc *response.RequestContext)
	Delete(ae *env.AppEnv, rc *response.RequestContext)
}

type ReadOnlyRouter interface {
	List(ae *env.AppEnv, rc *response.RequestContext)
	Detail(ae *env.AppEnv, rc *response.RequestContext)
}

type ReadUpdateRouter interface {
	ReadOnlyRouter
	Update(ae *env.AppEnv, rc *response.RequestContext)
	Patch(ae *env.AppEnv, rc *response.RequestContext)
}

type ModelToApiMapper func(*env.AppEnv, *response.RequestContext, models.Entity) (interface{}, error)

func GetModelQueryOptionsFromRequest(r *http.Request) (*QueryOptions, error) {
	filter := r.URL.Query().Get("filter")
	sort := r.URL.Query().Get("sort")

	pg, err := GetRequestPaging(r)

	if err != nil {
		return nil, err
	}

	return &QueryOptions{
		Predicate: filter,
		Sort:      sort,
		Paging:    pg,
	}, nil
}

func GetRequestPaging(r *http.Request) (*predicate.Paging, error) {
	l := r.URL.Query().Get("limit")
	o := r.URL.Query().Get("offset")

	var p *predicate.Paging

	if l != "" {
		i, err := strconv.ParseInt(l, 10, 64)

		if err != nil {
			return nil, &apierror.ApiError{
				Code:        apierror.InvalidPaginationCode,
				Message:     apierror.InvalidPaginationMessage,
				Cause:       apierror.NewFieldError("could not parse limit, value is not an integer", "limit", l),
				AppendCause: true,
			}
		}
		p = &predicate.Paging{}
		p.Limit = i
	}

	if o != "" {
		i, err := strconv.ParseInt(o, 10, 64)

		if err != nil {
			return nil, &apierror.ApiError{
				Code:        apierror.InvalidPaginationCode,
				Message:     apierror.InvalidPaginationMessage,
				Cause:       apierror.NewFieldError("could not parse offset, value is not an integer", "offset", o),
				AppendCause: true,
			}
		}
		if p == nil {
			p = &predicate.Paging{}
		}
		p.Offset = i
	}

	return p, nil
}

type ApiCreater interface {
	Create(ae *env.AppEnv, rc *response.RequestContext) (migration.BaseDbModel, error)
}

type ApiUpdater interface {
	Update(ae *env.AppEnv, rc *response.RequestContext, existing migration.BaseDbModel) (migration.BaseDbModel, error)
}

type ApiPatcher interface {
	Patch(ae *env.AppEnv, rc *response.RequestContext, existing migration.BaseDbModel) (migration.BaseDbModel, error)
}

type Associater interface {
	Add(parent migration.BaseDbModel, child []migration.BaseDbModel) error
	Remove(parent migration.BaseDbModel, child []migration.BaseDbModel) error
	Set(parent migration.BaseDbModel, child []migration.BaseDbModel) error
}

type RouteEventContext struct {
	AppEnv         *env.AppEnv
	RequestContext *response.RequestContext
}

type DeleteEventHandler interface {
	BeforeStoreDelete(rc *RouteEventContext, id string) error
}

type CreateEventHandler interface {
	BeforeStoreCreate(rc *RouteEventContext, modelEntity interface{}) error
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
