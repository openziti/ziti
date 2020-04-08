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
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/controller/schema"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/validation"
	"github.com/xeipuuv/gojsonschema"
	"io/ioutil"
	"strings"
)

const (
	EntityNameSelf = "self"
)

func unmarshal(body []byte, in interface{}) error {
	err := json.Unmarshal(body, in)

	if err != nil {
		err = apierror.GetJsonParseError(err, body)
	}

	return err
}

type JsonFields map[string]bool

func (j JsonFields) IsUpdated(key string) bool {
	_, ok := j[key]
	return ok
}

func (j JsonFields) AddField(key string) {
	j[key] = true
}

func (j JsonFields) ConcatNestedNames() JsonFields {
	for key, val := range j {
		if strings.Contains(key, ".") {
			delete(j, key)
			key = strings.ReplaceAll(key, ".", "")
			j[key] = val
		}
	}
	return j
}

func (j JsonFields) FilterMaps(mapNames ...string) JsonFields {
	nameMap := map[string]string{}
	for _, name := range mapNames {
		nameMap[name] = name + "."
	}
	for key := range j {
		for name, dotName := range nameMap {
			if strings.HasPrefix(key, dotName) {
				delete(j, key)
				j[name] = true
				break
			}
		}
	}
	return j
}

func getFields(body []byte) (JsonFields, error) {
	jsonMap := map[string]interface{}{}
	err := json.Unmarshal(body, &jsonMap)

	if err != nil {
		return nil, apierror.GetJsonParseError(err, body)
	}

	resultMap := JsonFields{}
	getJsonFields("", jsonMap, resultMap)
	return resultMap, nil
}

func getJsonFields(prefix string, m map[string]interface{}, result JsonFields) {
	for k, v := range m {
		name := k
		if subMap, ok := v.(map[string]interface{}); ok {
			getJsonFields(prefix+name+".", subMap, result)
		} else {
			isSet := v != nil
			result[prefix+name] = isSet
		}
	}
}

func modelToApi(ae *env.AppEnv, rc *response.RequestContext, mapper ModelToApiMapper, es []models.Entity) ([]BaseApiEntity, error) {
	apiEntities := make([]BaseApiEntity, 0)

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
	List(rc, func(rc *response.RequestContext, queryOptions *QueryOptions) (*QueryResult, error) {
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

type modelListF func(rc *response.RequestContext, queryOptions *QueryOptions) (*QueryResult, error)

func List(rc *response.RequestContext, f modelListF) {
	qo, err := GetModelQueryOptionsFromRequest(rc.Request)

	if err != nil {
		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not build query options")
		rc.RequestResponder.RespondWithError(err)
		return
	}

	result, err := f(rc, qo)

	if err != nil {
		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not convert list")
		rc.RequestResponder.RespondWithError(err)
		return
	}

	if result.Result == nil {
		result.Result = []BaseApiEntity{}
	}

	meta := &response.Meta{
		"pagination": map[string]interface{}{
			"limit":      result.Limit,
			"offset":     result.Offset,
			"totalCount": result.Count,
		},
		"filterableFields": result.FilterableFields,
	}

	rc.RequestResponder.RespondWithOk(result.Result, meta)
}

type ModelCreateF func() (string, error)

func Create(rc *response.RequestContext, rr response.RequestResponder, sc *gojsonschema.Schema, in interface{}, lb LinkBuilder, creator ModelCreateF) {
	var body []byte
	var err error

	if body, err = ioutil.ReadAll(rc.Request.Body); err != nil {
		rr.RespondWithCouldNotReadBody(err)
		return
	}

	if err = unmarshal(body, in); err != nil {
		rr.RespondWithCouldNotParseBody(err)
		return
	}

	il := gojsonschema.NewBytesLoader(body)

	result, err := sc.Validate(il)

	if err != nil {
		rr.RespondWithError(err)
		return
	}

	if !result.Valid() {
		rr.RespondWithValidationErrors(schema.NewValidationErrors(result))
		return
	}

	id, err := creator()
	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rr.RespondWithNotFound()
			return
		}

		if fe, ok := err.(*validation.FieldError); ok {
			rr.RespondWithFieldError(fe)
			return
		}

		if sve, ok := err.(*schema.ValidationErrors); ok {
			rr.RespondWithValidationErrors(sve)
			return
		}

		rr.RespondWithError(err)
		return
	}

	rr.RespondWithCreatedId(id, lb(id))
}

func DetailWithHandler(ae *env.AppEnv, rc *response.RequestContext, loader models.EntityRetriever, mapper ModelToApiMapper, idType response.IdType) {
	Detail(rc, idType, func(rc *response.RequestContext, id string) (interface{}, error) {
		entity, err := loader.BaseLoad(id)
		if err != nil {
			return nil, err
		}
		return mapper(ae, rc, entity)
	})
}

type ModelDetailF func(rc *response.RequestContext, id string) (interface{}, error)

func Detail(rc *response.RequestContext, idType response.IdType, f ModelDetailF) {
	id, err := rc.GetIdFromRequest(idType)

	if err != nil {
		pfxlog.Logger().Error(err)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	apiEntity, err := f(rc, id)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		pfxlog.Logger().WithField("id", id).WithError(err).Error("could not load entity by id")
		rc.RequestResponder.RespondWithError(err)
		return
	}

	rc.RequestResponder.RespondWithOk(apiEntity, nil)
}

type ModelDeleteF func(rc *response.RequestContext, id string) error

type DeleteHandler interface {
	Delete(id string) error
}

func DeleteWithHandler(rc *response.RequestContext, idType response.IdType, deleteHandler DeleteHandler) {
	Delete(rc, idType, func(rc *response.RequestContext, id string) error {
		return deleteHandler.Delete(id)
	})
}

func Delete(rc *response.RequestContext, idType response.IdType, deleteF ModelDeleteF) {
	id, err := rc.GetIdFromRequest(idType)

	if err != nil {
		log := pfxlog.Logger()
		log.Error(err)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	err = deleteF(rc, id)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
		} else {
			rc.RequestResponder.RespondWithError(err)
		}
		return
	}

	rc.RequestResponder.RespondWithOk(nil, nil)
}

type ModelUpdateF func(id string) error

func Update(rc *response.RequestContext, sc *gojsonschema.Schema, idType response.IdType, in interface{}, updateF ModelUpdateF) {
	UpdateAllowEmptyBody(rc, sc, idType, in, false, updateF)
}

func UpdateAllowEmptyBody(rc *response.RequestContext, sc *gojsonschema.Schema, idType response.IdType, in interface{}, emptyBodyAllowed bool, updateF ModelUpdateF) {
	id, err := rc.GetIdFromRequest(idType)

	if err != nil {
		log := pfxlog.Logger()
		log.Error(err)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	var body []byte

	if body, err = ioutil.ReadAll(rc.Request.Body); err != nil {
		rc.RequestResponder.RespondWithCouldNotReadBody(err)
		return
	}

	if len(body) > 0 || !emptyBodyAllowed {
		if err = unmarshal(body, in); err != nil {
			rc.RequestResponder.RespondWithCouldNotParseBody(err)
			return
		}

		il := gojsonschema.NewBytesLoader(body)

		result, err := sc.Validate(il)

		if err != nil {
			rc.RequestResponder.RespondWithError(err)
			return
		}

		if !result.Valid() {
			rc.RequestResponder.RespondWithValidationErrors(schema.NewValidationErrors(result))
			return
		}
	}

	if err = updateF(id); err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		if fe, ok := err.(*validation.FieldError); ok {
			rc.RequestResponder.RespondWithFieldError(fe)
			return
		}

		if sve, ok := err.(*schema.ValidationErrors); ok {
			rc.RequestResponder.RespondWithValidationErrors(sve)
			return
		}

		rc.RequestResponder.RespondWithError(err)
		return
	}

	rc.RequestResponder.RespondWithOk(nil, nil)
}

type ModelPatchF func(id string, fields JsonFields) error

func Patch(rc *response.RequestContext, sc *gojsonschema.Schema, idType response.IdType, in interface{}, patchF ModelPatchF) {
	id, err := rc.GetIdFromRequest(idType)

	if err != nil {
		log := pfxlog.Logger()
		log.Error(err)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	var body []byte

	if body, err = ioutil.ReadAll(rc.Request.Body); err != nil {
		rc.RequestResponder.RespondWithCouldNotReadBody(err)
		return
	}

	if err = unmarshal(body, in); err != nil {
		rc.RequestResponder.RespondWithCouldNotParseBody(err)
		return
	}

	jsonFields, err := getFields(body)
	if err != nil {
		rc.RequestResponder.RespondWithCouldNotParseBody(err)
	}

	il := gojsonschema.NewBytesLoader(body)

	result, err := sc.Validate(il)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	if !result.Valid() {
		rc.RequestResponder.RespondWithValidationErrors(schema.NewValidationErrors(result))
		return
	}

	err = patchF(id, jsonFields)
	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		if fe, ok := err.(*validation.FieldError); ok {
			rc.RequestResponder.RespondWithFieldError(fe)
			return
		}

		if sve, ok := err.(*schema.ValidationErrors); ok {
			rc.RequestResponder.RespondWithValidationErrors(sve)
			return
		}

		rc.RequestResponder.RespondWithError(err)
		return
	}

	rc.RequestResponder.RespondWithOk(nil, nil)
}

func listWithId(rc *response.RequestContext, idType response.IdType, f func(id string) ([]interface{}, error)) {
	id, err := rc.GetIdFromRequest(idType)

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not find id property: %v", response.IdPropertyName)
		log.WithField("property", response.IdPropertyName).Error(logErr)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	results, err := f(id)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		log := pfxlog.Logger()
		log.WithField("id", id).WithError(err).Error("could not load associations by id")
		rc.RequestResponder.RespondWithError(err)
		return
	}

	count := len(results)

	meta := &response.Meta{
		"pagination": map[string]interface{}{
			"limit":      count,
			"offset":     0,
			"totalCount": count,
		},
		"filterableFields": []string{},
	}

	rc.RequestResponder.RespondWithOk(results, meta)
}

// type ListAssocF func(string, func(models.Entity)) error
type listAssocF func(rc *response.RequestContext, id string, queryOptions *QueryOptions) (*QueryResult, error)

func ListAssociationsWithFilter(ae *env.AppEnv, rc *response.RequestContext, idType response.IdType, filterTemplate string, entityController models.EntityRetriever, mapper ModelToApiMapper) {
	ListAssociations(rc, idType, func(rc *response.RequestContext, id string, queryOptions *QueryOptions) (*QueryResult, error) {
		query, err := queryOptions.getFullQuery(entityController.GetStore())
		if err != nil {
			return nil, err
		}

		filter := fmt.Sprintf(filterTemplate, id)

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

func ListAssociationWithHandler(ae *env.AppEnv, rc *response.RequestContext, idType response.IdType, lister models.EntityRetriever, associationLoader models.EntityRetriever, mapper ModelToApiMapper) {
	ListAssociations(rc, idType, func(rc *response.RequestContext, id string, queryOptions *QueryOptions) (*QueryResult, error) {
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

func ListAssociations(rc *response.RequestContext, idType response.IdType, listF listAssocF) {
	id, err := rc.GetIdFromRequest(idType)

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not find id property: %v", response.IdPropertyName)
		log.WithField("property", response.IdPropertyName).Error(logErr)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	filter := rc.Request.URL.Query().Get("filter")
	queryOptions := &QueryOptions{
		Predicate: filter,
	}

	result, err := listF(rc, id, queryOptions)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not convert list")
		rc.RequestResponder.RespondWithError(err)
		return
	}

	if result.Result == nil {
		result.Result = []BaseApiEntity{}
	}

	meta := &response.Meta{
		"pagination": map[string]interface{}{
			"limit":      result.Limit,
			"offset":     result.Offset,
			"totalCount": result.Count,
		},
		"filterableFields": result.FilterableFields,
	}

	rc.RequestResponder.RespondWithOk(result.Result, meta)
}
