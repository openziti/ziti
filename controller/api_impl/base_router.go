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

package api_impl

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/rest_model"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"net/http"
	"reflect"
)

const (
	EntityNameSelf = "self"
)

func modelToApi(network *network.Network, rc api.RequestContext, mapper ModelToApiMapper, es []models.Entity) ([]interface{}, error) {
	apiEntities := make([]interface{}, 0)

	for _, e := range es {
		al, err := mapper(network, rc, e)

		if err != nil {
			return nil, err
		}

		apiEntities = append(apiEntities, al)
	}

	return apiEntities, nil
}

func ListWithHandler(n *network.Network, rc api.RequestContext, lister models.EntityRetriever, mapper ModelToApiMapper) {
	ListWithQueryF(n, rc, lister, mapper, lister.BasePreparedList)
}

type queryF func(query ast.Query) (*models.EntityListResult, error)

func ListWithQueryF(n *network.Network, rc api.RequestContext, lister models.EntityRetriever, mapper ModelToApiMapper, qf queryF) {
	ListWithQueryFAndCollector(n, rc, lister, mapper, defaultToListEnvelope, qf)
}

func defaultToListEnvelope(data []interface{}, meta *rest_model.Meta) interface{} {
	return rest_model.Empty{
		Data: data,
		Meta: meta,
	}
}

type ApiListEnvelopeFactory func(data []interface{}, meta *rest_model.Meta) interface{}
type ApiEntityEnvelopeFactory func(data interface{}, meta *rest_model.Meta) interface{}

func ListWithQueryFAndCollector(n *network.Network, rc api.RequestContext, lister models.EntityRetriever, mapper ModelToApiMapper, toEnvelope ApiListEnvelopeFactory, qf queryF) {
	ListWithEnvelopeFactory(rc, toEnvelope, func(rc api.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {
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

		apiEntities, err := modelToApi(n, rc, mapper, result.GetEntities())
		if err != nil {
			return nil, err
		}

		return NewQueryResult(apiEntities, result.GetMetaData()), nil
	})
}

type modelListF func(rc api.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error)

func List(rc api.RequestContext, f modelListF) {
	ListWithEnvelopeFactory(rc, defaultToListEnvelope, f)
}

func ListWithEnvelopeFactory(rc api.RequestContext, toEnvelope ApiListEnvelopeFactory, f modelListF) {
	qo, err := GetModelQueryOptionsFromRequest(rc.GetRequest())

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

func Create(rc api.RequestContext, linkFactory CreateLinkFactory, creator ModelCreateF) {
	CreateWithResponder(rc, linkFactory, creator)
}

func CreateWithResponder(rsp api.Responder, linkFactory CreateLinkFactory, creator ModelCreateF) {
	id, err := creator()
	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rsp.RespondWithNotFoundWithCause(err)
			return
		}

		if fe, ok := err.(*errorz.FieldError); ok {
			rsp.RespondWithFieldError(fe)
			return
		}

		if sve, ok := err.(*apierror.ValidationErrors); ok {
			rsp.RespondWithValidationErrors(sve)
			return
		}

		rsp.RespondWithError(err)
		return
	}

	RespondWithCreatedId(rsp, id, linkFactory.SelfLinkFromId(id))
}

func DetailWithHandler(network *network.Network, rc api.RequestContext, loader models.EntityRetriever, mapper ModelToApiMapper) {
	Detail(rc, func(rc api.RequestContext, id string) (interface{}, error) {
		entity, err := loader.BaseLoad(id)
		if err != nil {
			return nil, err
		}
		return mapper(network, rc, entity)
	})
}

type ModelDetailF func(rc api.RequestContext, id string) (interface{}, error)

func Detail(rc api.RequestContext, f ModelDetailF) {
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

	RespondWithOk(rc, apiEntity, &rest_model.Meta{})
}

type ModelDeleteF func(rc api.RequestContext, id string) error

type DeleteHandler interface {
	Delete(id string) error
}

type DeleteHandlerF func(id string) error

func (self DeleteHandlerF) Delete(id string) error {
	return self(id)
}

func DeleteWithHandler(rc api.RequestContext, deleteHandler DeleteHandler) {
	Delete(rc, func(rc api.RequestContext, id string) error {
		return deleteHandler.Delete(id)
	})
}

func Delete(rc api.RequestContext, deleteF ModelDeleteF) {
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
		} else {
			rc.RespondWithError(err)
		}
		return
	}

	rc.RespondWithEmptyOk()
}

type ModelUpdateF func(id string) error

func Update(rc api.RequestContext, updateF ModelUpdateF) {
	UpdateAllowEmptyBody(rc, updateF)
}

func UpdateAllowEmptyBody(rc api.RequestContext, updateF ModelUpdateF) {
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

func Patch(rc api.RequestContext, patchF ModelPatchF) {
	id, err := rc.GetEntityId()

	if err != nil {
		log := pfxlog.Logger()
		log.Error(err)
		rc.RespondWithError(fmt.Errorf("error during patch, retrieving id: %v", err))
		return
	}

	jsonFields, err := api.GetFields(rc.GetBody())
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

// type ListAssocF func(string, func(models.Entity)) error
type listAssocF func(rc api.RequestContext, id string, queryOptions *PublicQueryOptions) (*QueryResult, error)

func ListAssociationWithHandler(n *network.Network, rc api.RequestContext, lister models.EntityRetriever, associationLoader models.EntityRetriever, mapper ModelToApiMapper) {
	ListAssociations(rc, func(rc api.RequestContext, id string, queryOptions *PublicQueryOptions) (*QueryResult, error) {
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

		apiEntities, err := modelToApi(n, rc, mapper, result.GetEntities())
		if err != nil {
			return nil, err
		}

		return NewQueryResult(apiEntities, result.GetMetaData()), nil
	})
}

func ListAssociations(rc api.RequestContext, listF listAssocF) {
	id, err := rc.GetEntityId()

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not find id property: %v", api.IdPropertyName)
		log.WithField("property", api.IdPropertyName).Error(logErr)
		rc.RespondWithError(err)
		return
	}

	queryOptions, err := GetModelQueryOptionsFromRequest(rc.GetRequest())

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

	RespondWithOk(rc, result.Result, meta)
}
