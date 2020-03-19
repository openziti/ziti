/*
	Copyright 2019 NetFoundry, Inc.

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
	"fmt"
	"github.com/gorilla/mux"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/predicate"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/migration"
	"github.com/netfoundry/ziti-fabric/controller/models"
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

type ModelToApiMapper func(*env.AppEnv, *response.RequestContext, models.Entity) (BaseApiEntity, error)

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

func GetRequestPredicate(r *http.Request, imap *predicate.IdentifierMap) (*predicate.Predicate, error) {
	var p *predicate.Predicate

	f := r.URL.Query().Get("filter")
	if f != "" {
		whereClause, errs := predicate.ParseWhereClause(f, imap)

		if len(errs) > 0 {
			return nil, &apierror.ApiError{
				Cause:   errs[0],
				Code:    apierror.InvalidFilterCode,
				Message: apierror.InvalidFilterMessage,
				Status:  http.StatusBadRequest,
			}
		}
		p = &predicate.Predicate{
			Clause: whereClause,
		}
	}

	return p, nil
}

func GetRequestSort(r *http.Request, imap *predicate.IdentifierMap) (*predicate.Sort, error) {
	s := r.URL.Query().Get("sort")

	so, err := predicate.ParseOrderBy(s, imap)

	if err != nil {
		return nil, &apierror.ApiError{
			Code:        apierror.InvalidSortCode,
			Message:     apierror.InvalidSortMessage,
			Cause:       err,
			AppendCause: true,
			Status:      http.StatusBadRequest,
		}
	}

	return so, nil
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

type EntityApiRef struct {
	Entity string          `json:"entity"`
	Id     string          `json:"id"`
	Name   *string         `json:"name"`
	Links  *response.Links `json:"_links"`
}

func (e *EntityApiRef) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"entity": e.Entity,
		"id":     e.Id,
		"name":   e.Name,
		"links":  e.Links,
	}
}

func (e *EntityApiRef) ToMapError(msg string) *apierror.GenericCauseError {
	return &apierror.GenericCauseError{
		Message: msg,
		DataMap: e.ToMap(),
	}
}

type BaseApiEntity interface {
	GetSelfLink() *response.Link
	BuildSelfLink(id string) *response.Link
	PopulateLinks()
	ToEntityApiRef() *EntityApiRef
}

type LinkBuilder func(id string) *response.Link

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

func registerCrudRouter(ae *env.AppEnv, r *mux.Router, basePath string, cr CrudRouter, pr permissions.Resolver) *mux.Router {
	rpr := pr
	cpr := pr
	dpr := pr
	upr := pr

	crs, ok := pr.(*crudResolvers)

	if ok {
		rpr = crs.GetReadResolver()
		cpr = crs.GetCreateResolver()
		dpr = crs.GetDeleteResolver()
		upr = crs.GetUpdateResolver()
	}

	listHandler := ae.WrapHandler(cr.List, rpr)
	detailHandler := ae.WrapHandler(cr.Detail, rpr)
	createHandler := ae.WrapHandler(cr.Create, cpr)
	deleteHandler := ae.WrapHandler(cr.Delete, dpr)
	updateHandler := ae.WrapHandler(cr.Update, upr)
	patchHandler := ae.WrapHandler(cr.Patch, upr)

	s := r.PathPrefix(basePath).Subrouter()
	s.HandleFunc("", listHandler).Methods(http.MethodGet)
	s.HandleFunc("/", listHandler).Methods(http.MethodGet)

	s.HandleFunc("", createHandler).Methods(http.MethodPost)
	s.HandleFunc("/", createHandler).Methods(http.MethodPost)

	idUrlWithoutSlash := fmt.Sprintf("/{%s}", response.IdPropertyName)
	idUrlWithSlash := fmt.Sprintf("/{%s}/", response.IdPropertyName)

	s.HandleFunc(idUrlWithoutSlash, detailHandler).Methods(http.MethodGet)
	s.HandleFunc(idUrlWithSlash, detailHandler).Methods(http.MethodGet)

	s.HandleFunc(idUrlWithoutSlash, patchHandler).Methods(http.MethodPatch)
	s.HandleFunc(idUrlWithSlash, patchHandler).Methods(http.MethodPatch)

	s.HandleFunc(idUrlWithoutSlash, updateHandler).Methods(http.MethodPut)
	s.HandleFunc(idUrlWithSlash, updateHandler).Methods(http.MethodPut)

	s.HandleFunc(idUrlWithoutSlash, deleteHandler).Methods(http.MethodDelete)
	s.HandleFunc(idUrlWithSlash, deleteHandler).Methods(http.MethodDelete)

	return s
}

func registerReadUpdateRouter(ae *env.AppEnv, r *mux.Router, basePath string, cr ReadUpdateRouter, pr permissions.Resolver) *mux.Router {
	rpr := pr
	upr := pr

	crs, ok := pr.(*crudResolvers)

	if ok {
		rpr = crs.GetReadResolver()
		upr = crs.GetUpdateResolver()
	}

	listHandler := ae.WrapHandler(cr.List, rpr)
	detailHandler := ae.WrapHandler(cr.Detail, rpr)
	updateHandler := ae.WrapHandler(cr.Update, upr)
	patchHandler := ae.WrapHandler(cr.Patch, upr)

	s := r.PathPrefix(basePath).Subrouter()
	s.HandleFunc("", listHandler).Methods(http.MethodGet)
	s.HandleFunc("/", listHandler).Methods(http.MethodGet)

	idUrlWithoutSlash := fmt.Sprintf("/{%s}", response.IdPropertyName)
	idUrlWithSlash := fmt.Sprintf("/{%s}/", response.IdPropertyName)

	s.HandleFunc(idUrlWithoutSlash, detailHandler).Methods(http.MethodGet)
	s.HandleFunc(idUrlWithSlash, detailHandler).Methods(http.MethodGet)

	s.HandleFunc(idUrlWithoutSlash, patchHandler).Methods(http.MethodPatch)
	s.HandleFunc(idUrlWithSlash, patchHandler).Methods(http.MethodPatch)

	s.HandleFunc(idUrlWithoutSlash, updateHandler).Methods(http.MethodPut)
	s.HandleFunc(idUrlWithSlash, updateHandler).Methods(http.MethodPut)

	return s
}

func registerReadOnlyRouter(ae *env.AppEnv, r *mux.Router, basePath string, ro ReadOnlyRouter, pr permissions.Resolver) *mux.Router {
	rpr := pr

	crs, ok := pr.(*crudResolvers)

	if ok {
		rpr = crs.GetReadResolver()
	}

	listHandler := ae.WrapHandler(ro.List, rpr)
	detailHandler := ae.WrapHandler(ro.Detail, rpr)

	s := r.PathPrefix(basePath).Subrouter()
	s.HandleFunc("", listHandler).Methods(http.MethodGet)
	s.HandleFunc("/", listHandler).Methods(http.MethodGet)

	idUrlWithoutSlash := fmt.Sprintf("/{%s}", response.IdPropertyName)
	idUrlWithSlash := fmt.Sprintf("/{%s}/", response.IdPropertyName)

	s.HandleFunc(idUrlWithoutSlash, detailHandler).Methods(http.MethodGet)
	s.HandleFunc(idUrlWithSlash, detailHandler).Methods(http.MethodGet)
	return s
}

func registerReadDeleteOnlyRouter(ae *env.AppEnv, r *mux.Router, basePath string, rdr ReadDeleteRouter, pr permissions.Resolver) *mux.Router {
	rpr := pr
	dpr := pr

	crs, ok := pr.(*crudResolvers)

	if ok {
		rpr = crs.GetReadResolver()
		dpr = crs.GetDeleteResolver()
	}

	listHandler := ae.WrapHandler(rdr.List, rpr)
	detailHandler := ae.WrapHandler(rdr.Detail, rpr)
	deleteHandler := ae.WrapHandler(rdr.Delete, dpr)

	s := r.PathPrefix(basePath).Subrouter()
	s.HandleFunc("", listHandler).Methods(http.MethodGet)
	s.HandleFunc("/", listHandler).Methods(http.MethodGet)

	idUrlWithoutSlash := fmt.Sprintf("/{%s}", response.IdPropertyName)
	idUrlWithSlash := fmt.Sprintf("/{%s}/", response.IdPropertyName)

	s.HandleFunc(idUrlWithoutSlash, detailHandler).Methods(http.MethodGet)
	s.HandleFunc(idUrlWithSlash, detailHandler).Methods(http.MethodGet)

	s.HandleFunc(idUrlWithoutSlash, deleteHandler).Methods(http.MethodDelete)
	s.HandleFunc(idUrlWithSlash, deleteHandler).Methods(http.MethodDelete)

	return s
}

func registerCreateReadDeleteRouter(ae *env.AppEnv, r *mux.Router, basePath string, crd CreateReadDeleteOnlyRouter, cr permissions.Resolver) {
	rpr := cr
	cpr := cr
	dpr := cr

	crs, ok := cr.(*crudResolvers)

	if ok {
		rpr = crs.GetReadResolver()
		cpr = crs.GetCreateResolver()
		dpr = crs.GetDeleteResolver()
	}

	listHandler := ae.WrapHandler(crd.List, rpr)
	detailHandler := ae.WrapHandler(crd.Detail, rpr)
	createHandler := ae.WrapHandler(crd.Create, cpr)
	deleteHandler := ae.WrapHandler(crd.Delete, dpr)

	s := r.PathPrefix(basePath).Subrouter()
	s.HandleFunc("", listHandler).Methods(http.MethodGet)
	s.HandleFunc("/", listHandler).Methods(http.MethodGet)

	s.HandleFunc("", createHandler).Methods(http.MethodPost)
	s.HandleFunc("/", createHandler).Methods(http.MethodPost)

	idUrlWithoutSlash := fmt.Sprintf("/{%s}", response.IdPropertyName)
	idUrlWithSlash := fmt.Sprintf("/{%s}/", response.IdPropertyName)

	s.HandleFunc(idUrlWithoutSlash, detailHandler).Methods(http.MethodGet)
	s.HandleFunc(idUrlWithSlash, detailHandler).Methods(http.MethodGet)

	s.HandleFunc(idUrlWithoutSlash, deleteHandler).Methods(http.MethodDelete)
	s.HandleFunc(idUrlWithSlash, deleteHandler).Methods(http.MethodDelete)

}

type crudResolvers struct {
	Default permissions.Resolver
	Create  permissions.Resolver
	Read    permissions.Resolver
	Update  permissions.Resolver
	Delete  permissions.Resolver
}

func (crs *crudResolvers) GetCreateResolver() permissions.Resolver {
	if crs.Create == nil {
		return crs.Default
	}

	return crs.Create
}

func (crs *crudResolvers) GetReadResolver() permissions.Resolver {
	if crs.Read == nil {
		return crs.Default
	}

	return crs.Read
}

func (crs *crudResolvers) GetUpdateResolver() permissions.Resolver {
	if crs.Update == nil {
		return crs.Default
	}

	return crs.Update
}

func (crs *crudResolvers) GetDeleteResolver() permissions.Resolver {
	if crs.Delete == nil {
		return crs.Default
	}

	return crs.Delete
}

func (crs *crudResolvers) IsAllowed(args ...string) bool {
	return crs.Default.IsAllowed(args...)
}

type QueryResult struct {
	Result           []BaseApiEntity
	Count            int64
	Limit            int64
	Offset           int64
	FilterableFields []string
}

func NewQueryResult(result []BaseApiEntity, metadata *models.QueryMetaData) *QueryResult {
	return &QueryResult{
		Result:           result,
		Count:            metadata.Count,
		Limit:            metadata.Limit,
		Offset:           metadata.Offset,
		FilterableFields: metadata.FilterableFields,
	}
}
