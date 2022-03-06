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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	edgeApiError "github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"net/http"
	"reflect"
	"strings"
)

const (
	EntityNameSelf = "self"
)

func modelToApi(ae *env.AppEnv, rc *response.RequestContext, mapper ModelToApiMapper, es []models.Entity) ([]interface{}, error) {
	apiEntities := make([]interface{}, 0)

	for _, e := range es {
		al, err := mapper(ae, rc, e)

		if err != nil {
			return nil, err
		}

		apiEntities = append(apiEntities, al)
	}

	return apiEntities, nil
}

func ListWithHandler(ae *env.AppEnv, rc *response.RequestContext, lister models.EntityRetriever, mapper ModelToApiMapper) {
	ListWithQueryF(ae, rc, lister, mapper, lister.BasePreparedList)
}

type queryF func(query ast.Query) (*models.EntityListResult, error)

func ListWithQueryF(ae *env.AppEnv, rc *response.RequestContext, lister models.EntityRetriever, mapper ModelToApiMapper, qf queryF) {
	ListWithQueryFAndCollector(ae, rc, lister, mapper, defaultToListEnvelope, qf)
}

func defaultToListEnvelope(data []interface{}, meta *rest_model.Meta) interface{} {
	return rest_model.Empty{
		Data: data,
		Meta: meta,
	}
}

type ApiListEnvelopeFactory func(data []interface{}, meta *rest_model.Meta) interface{}
type ApiEntityEnvelopeFactory func(data interface{}, meta *rest_model.Meta) interface{}

func ListWithQueryFAndCollector(ae *env.AppEnv, rc *response.RequestContext, lister models.EntityRetriever, mapper ModelToApiMapper, toEnvelope ApiListEnvelopeFactory, qf queryF) {
	ListWithEnvelopeFactory(rc, toEnvelope, func(rc *response.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {
		// validate that the submitted query is only using public symbols. The query options may contain an final
		// query which has been modified with additional filters
		query, err := queryOptions.getFullQuery(lister.GetStore())
		if err != nil {
			return nil, err
		}

		result, err := qf(query)
		if err != nil {
			return nil, err
		}

		apiEntities, err := modelToApi(ae, rc, mapper, result.GetEntities())
		if err != nil {
			return nil, err
		}

		return NewQueryResult(apiEntities, result.GetMetaData()), nil
	})
}

type modelListF func(rc *response.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error)

func List(rc *response.RequestContext, f modelListF) {
	ListWithEnvelopeFactory(rc, defaultToListEnvelope, f)
}

func ListWithEnvelopeFactory(rc *response.RequestContext, toEnvelope ApiListEnvelopeFactory, f modelListF) {
	qo, err := GetModelQueryOptionsFromRequest(rc.Request)

	if err != nil {
		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not build query options")
		rc.RespondWithError(err)
		return
	}

	result, err := f(rc, qo)

	if err != nil {
		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not convert list")
		rc.RespondWithError(err)
		return
	}

	if result.Result == nil {
		result.Result = []interface{}{}
	}

	meta := &rest_model.Meta{
		Pagination: &rest_model.Pagination{
			Limit:      &result.Limit,
			Offset:     &result.Offset,
			TotalCount: &result.Count,
		},
		FilterableFields: result.FilterableFields,
	}

	switch reflect.TypeOf(result.Result).Kind() {
	case reflect.Slice:
		slice := reflect.ValueOf(result.Result)

		//noinspection GoPreferNilSlice
		elements := []interface{}{}
		for i := 0; i < slice.Len(); i++ {
			elem := slice.Index(i)
			elements = append(elements, elem.Interface())
		}

		envelope := toEnvelope(elements, meta)
		rc.Respond(envelope, http.StatusOK)
	default:
		envelope := toEnvelope([]interface{}{result.Result}, meta)
		rc.Respond(envelope, http.StatusOK)
	}
}

type ModelCreateF func() (string, error)

func Create(rc *response.RequestContext, _ response.Responder, linkFactory CreateLinkFactory, creator ModelCreateF) {
	CreateWithResponder(rc, rc, linkFactory, creator)
}

func CreateWithResponder(rc *response.RequestContext, rsp response.Responder, linkFactory CreateLinkFactory, creator ModelCreateF) {
	id, err := creator()
	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RespondWithNotFoundWithCause(err)
			return
		}

		if fe, ok := err.(*errorz.FieldError); ok {
			rc.RespondWithFieldError(fe)
			return
		}

		if sve, ok := err.(*apierror.ValidationErrors); ok {
			rc.RespondWithValidationErrors(sve)
			return
		}

		if uie, ok := err.(*boltz.UniqueIndexDuplicateError); ok {
			rc.RespondWithFieldError(&errorz.FieldError{
				Reason:     uie.Error(),
				FieldName:  uie.Field,
				FieldValue: uie.Value,
			})
			return
		}

		rc.RespondWithError(err)
		return
	}

	rsp.RespondWithCreatedId(id, linkFactory.SelfLinkFromId(id))
}

func DetailWithHandler(ae *env.AppEnv, rc *response.RequestContext, loader models.EntityRetriever, mapper ModelToApiMapper) {
	Detail(rc, func(rc *response.RequestContext, id string) (interface{}, error) {
		entity, err := loader.BaseLoad(id)
		if err != nil {
			return nil, err
		}
		return mapper(ae, rc, entity)
	})
}

type ModelDetailF func(rc *response.RequestContext, id string) (interface{}, error)

func Detail(rc *response.RequestContext, f ModelDetailF) {
	id, err := rc.GetEntityId()

	if err != nil {
		pfxlog.Logger().Error(err)
		rc.RespondWithError(err)
		return
	}

	apiEntity, err := f(rc, id)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RespondWithNotFoundWithCause(err)
			return
		}

		pfxlog.Logger().WithField("id", id).WithError(err).Error("could not load entity by id")
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithOk(apiEntity, &rest_model.Meta{})
}

type ModelDeleteF func(rc *response.RequestContext, id string) error

type DeleteHandler interface {
	Delete(id string) error
}

func DeleteWithHandler(rc *response.RequestContext, deleteHandler DeleteHandler) {
	Delete(rc, func(rc *response.RequestContext, id string) error {
		return deleteHandler.Delete(id)
	})
}

func Delete(rc *response.RequestContext, deleteF ModelDeleteF) {
	id, err := rc.GetEntityId()

	if err != nil {
		log := pfxlog.Logger()
		log.Error(err)
		rc.RespondWithError(err)
		return
	}

	err = deleteF(rc, id)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RespondWithNotFoundWithCause(err)
		} else if refErr, ok := err.(*boltz.ReferenceExistsError); ok {
			rc.RespondWithApiError(edgeApiError.NewCanNotDeleteReferencedEntity(refErr.LocalType, refErr.RemoteType, refErr.RemoteIds, refErr.RemoteField))
		} else {
			rc.RespondWithError(err)
		}
		return
	}

	rc.RespondWithEmptyOk()
}

type ModelUpdateF func(id string) error

func Update(rc *response.RequestContext, updateF ModelUpdateF) {
	UpdateAllowEmptyBody(rc, updateF)
}

func UpdateAllowEmptyBody(rc *response.RequestContext, updateF ModelUpdateF) {
	id, err := rc.GetEntityId()

	if err != nil {
		log := pfxlog.Logger()
		log.Error(err)
		rc.RespondWithError(fmt.Errorf("error during update, retrieving id: %v", err))
		return
	}

	if err = updateF(id); err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RespondWithNotFoundWithCause(err)
			return
		}

		if fe, ok := err.(*errorz.FieldError); ok {
			rc.RespondWithFieldError(fe)
			return
		}

		if sve, ok := err.(*apierror.ValidationErrors); ok {
			rc.RespondWithValidationErrors(sve)
			return
		}

		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}

type ModelPatchF func(id string, fields api.JsonFields) error

func Patch(rc *response.RequestContext, patchF ModelPatchF) {
	id, err := rc.GetEntityId()

	if err != nil {
		log := pfxlog.Logger()
		log.Error(err)
		rc.RespondWithError(fmt.Errorf("error during patch, retrieving id: %v", err))
		return
	}

	jsonFields, err := api.GetFields(rc.Body)
	if err != nil {
		rc.RespondWithCouldNotParseBody(err)
	}

	err = patchF(id, jsonFields)
	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RespondWithNotFoundWithCause(err)
			return
		}

		if fe, ok := err.(*errorz.FieldError); ok {
			rc.RespondWithFieldError(fe)
			return
		}

		if sve, ok := err.(*apierror.ValidationErrors); ok {
			rc.RespondWithValidationErrors(sve)
			return
		}

		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}

func listWithId(rc *response.RequestContext, f func(id string) ([]interface{}, error)) {
	id, err := rc.GetEntityId()

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not find id property: %v", response.IdPropertyName)
		log.WithField("property", response.IdPropertyName).Error(logErr)
		rc.RespondWithError(err)
		return
	}

	results, err := f(id)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RespondWithNotFoundWithCause(err)
			return
		}

		log := pfxlog.Logger()
		log.WithField("id", id).WithError(err).Error("could not load associations by id")
		rc.RespondWithError(err)
		return
	}

	count := len(results)

	limit := int64(count)
	offset := int64(0)
	totalCount := int64(count)

	meta := &rest_model.Meta{
		FilterableFields: []string{},
		Pagination: &rest_model.Pagination{
			Limit:      &limit,
			Offset:     &offset,
			TotalCount: &totalCount,
		},
	}

	rc.RespondWithOk(results, meta)
}

// type ListAssocF func(string, func(models.Entity)) error
type listAssocF func(rc *response.RequestContext, id string, queryOptions *PublicQueryOptions) (*QueryResult, error)

func ListAssociationsWithFilter(ae *env.AppEnv, rc *response.RequestContext, filterTemplate string, entityController models.EntityRetriever, mapper ModelToApiMapper) {
	ListAssociations(rc, func(rc *response.RequestContext, id string, queryOptions *PublicQueryOptions) (*QueryResult, error) {
		query, err := queryOptions.getFullQuery(entityController.GetStore())
		if err != nil {
			return nil, err
		}

		filter := filterTemplate
		if strings.Contains(filterTemplate, "%v") || strings.Contains(filterTemplate, "%s") {
			filter = fmt.Sprintf(filterTemplate, id)
		}

		filterQuery, err := ast.Parse(entityController.GetStore(), filter)
		if err != nil {
			return nil, err
		}

		query.SetPredicate(ast.NewAndExprNode(query.GetPredicate(), filterQuery.GetPredicate()))

		result, err := entityController.BasePreparedList(query)
		if err != nil {
			return nil, err
		}

		entities, err := modelToApi(ae, rc, mapper, result.GetEntities())
		if err != nil {
			return nil, err
		}

		return NewQueryResult(entities, &result.QueryMetaData), nil
	})
}

func ListAssociationWithHandler(ae *env.AppEnv, rc *response.RequestContext, lister models.EntityRetriever, associationLoader models.EntityRetriever, mapper ModelToApiMapper) {
	ListAssociations(rc, func(rc *response.RequestContext, id string, queryOptions *PublicQueryOptions) (*QueryResult, error) {
		// validate that the submitted query is only using public symbols. The query options may contain an final
		// query which has been modified with additional filters
		query, err := queryOptions.getFullQuery(associationLoader.GetStore())
		if err != nil {
			return nil, err
		}

		result, err := lister.BasePreparedListAssociated(id, associationLoader, query)
		if err != nil {
			return nil, err
		}

		apiEntities, err := modelToApi(ae, rc, mapper, result.GetEntities())
		if err != nil {
			return nil, err
		}

		return NewQueryResult(apiEntities, result.GetMetaData()), nil
	})
}

func ListAssociations(rc *response.RequestContext, listF listAssocF) {
	id, err := rc.GetEntityId()

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not find id property: %v", response.IdPropertyName)
		log.WithField("property", response.IdPropertyName).Error(logErr)
		rc.RespondWithError(err)
		return
	}

	queryOptions, err := GetModelQueryOptionsFromRequest(rc.Request)

	if err != nil {
		rc.RespondWithError(err)
	}

	result, err := listF(rc, id, queryOptions)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RespondWithNotFoundWithCause(err)
			return
		}

		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not convert list")
		rc.RespondWithError(err)
		return
	}

	if result.Result == nil {
		result.Result = []interface{}{}
	}

	meta := &rest_model.Meta{
		Pagination: &rest_model.Pagination{
			Limit:      &result.Limit,
			Offset:     &result.Offset,
			TotalCount: &result.Count,
		},
		FilterableFields: result.FilterableFields,
	}

	rc.RespondWithOk(result.Result, meta)
}
