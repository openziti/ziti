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

package routes

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/xeipuuv/gojsonschema"
	"io/ioutil"
	"strings"
)

const (
	EntityNameSelf = "self"
)

type associationIdArrayRequest struct {
	Ids []string `json:"ids"`
}

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
		name := strings.Title(k)
		if subMap, ok := v.(map[string]interface{}); ok {
			getJsonFields(prefix+name+".", subMap, result)
		} else {
			isSet := v != nil
			result[prefix+name] = isSet
		}
	}
}

func modelToApi(ae *env.AppEnv, rc *response.RequestContext, mapper ModelToApiMapper, es []model.BaseModelEntity) ([]BaseApiEntity, error) {
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

func ListWithHandler(ae *env.AppEnv, rc *response.RequestContext, handler model.Handler, mapper ModelToApiMapper) {
	List(rc, func(rc *response.RequestContext, queryOptions *model.QueryOptions) (*QueryResult, error) {
		result, err := handler.BaseList(queryOptions)
		if err != nil {
			return nil, err
		}
		apiEntities, err := modelToApi(ae, rc, mapper, result.Entities)
		if err != nil {
			return nil, err
		}
		return NewQueryResult(apiEntities, &result.QueryMetaData), nil
	})
}

type ModelListF func(rc *response.RequestContext, queryOptions *model.QueryOptions) (*QueryResult, error)

func List(rc *response.RequestContext, f ModelListF) {
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

	il := gojsonschema.NewGoLoader(in)

	result, err := sc.Validate(il)

	if err != nil {
		rr.RespondWithError(err)
		return
	}

	if !result.Valid() {
		var errs []*apierror.ValidationError
		for _, re := range result.Errors() {
			errs = append(errs, apierror.NewValidationError(re))
		}

		rr.RespondWithValidationErrors(errs)
		return
	}

	id, err := creator()
	if err != nil {
		if util.IsErrNotFoundErr(err) {
			rr.RespondWithNotFound()
			return
		}

		if fe, ok := err.(*model.FieldError); ok {
			rr.RespondWithFieldError(fe)
			return
		}

		rr.RespondWithError(err)
		return
	}

	rr.RespondWithCreatedId(id, lb(id))
}

func DetailWithHandler(ae *env.AppEnv, rc *response.RequestContext, handler model.Handler, mapper ModelToApiMapper, idType response.IdType) {
	Detail(rc, idType, func(rc *response.RequestContext, id string) (BaseApiEntity, error) {
		entity, err := handler.BaseLoad(id)
		if err != nil {
			return nil, err
		}
		return mapper(ae, rc, entity)
	})
}

type ModelDetailF func(rc *response.RequestContext, id string) (BaseApiEntity, error)

func Detail(rc *response.RequestContext, idType response.IdType, f ModelDetailF) {
	id, err := rc.GetIdFromRequest(idType)

	if err != nil {
		pfxlog.Logger().Error(err)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	apiEntity, err := f(rc, id)

	if err != nil {
		if util.IsErrNotFoundErr(err) {
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
	HandleDelete(id string) error
}

func DeleteWithHandler(rc *response.RequestContext, idType response.IdType, deleteHandler DeleteHandler) {
	Delete(rc, idType, func(rc *response.RequestContext, id string) error {
		return deleteHandler.HandleDelete(id)
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
		if util.IsErrNotFoundErr(err) {
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

	il := gojsonschema.NewGoLoader(in)

	result, err := sc.Validate(il)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	if !result.Valid() {
		var errs []*apierror.ValidationError
		for _, re := range result.Errors() {
			errs = append(errs, apierror.NewValidationError(re))
		}
		rc.RequestResponder.RespondWithValidationErrors(errs)
		return
	}

	if err = updateF(id); err != nil {
		if util.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		if fe, ok := err.(*model.FieldError); ok {
			rc.RequestResponder.RespondWithFieldError(fe)
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

	il := gojsonschema.NewGoLoader(in)

	result, err := sc.Validate(il)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	if !result.Valid() {
		var errs []*apierror.ValidationError
		for _, re := range result.Errors() {
			errs = append(errs, apierror.NewValidationError(re))
		}

		rc.RequestResponder.RespondWithValidationErrors(errs)
		return
	}

	err = patchF(id, jsonFields)
	if err != nil {
		if util.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		if fe, ok := err.(*model.FieldError); ok {
			rc.RequestResponder.RespondWithFieldError(fe)
			return
		}

		rc.RequestResponder.RespondWithError(err)
		return
	}

	rc.RequestResponder.RespondWithOk(nil, nil)
}

type ListAssocF func(string, func(model.BaseModelEntity)) error

func ListAssociations(ae *env.AppEnv, rc *response.RequestContext, idType response.IdType, listF ListAssocF, converter ModelToApiMapper) {
	id, err := rc.GetIdFromRequest(idType)

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not find id property: %v", response.IdPropertyName)
		log.WithField("property", response.IdPropertyName).Error(logErr)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	var modelResults []model.BaseModelEntity
	err = listF(id, func(entity model.BaseModelEntity) {
		modelResults = append(modelResults, entity)
	})

	if err != nil {
		if util.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		log := pfxlog.Logger()
		log.WithField("id", id).WithError(err).Error("could not load associations by id")
		rc.RequestResponder.RespondWithError(err)
		return
	}

	subApiEs, err := modelToApi(ae, rc, converter, modelResults)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	count := len(modelResults)

	meta := &response.Meta{
		"pagination": map[string]interface{}{
			"limit":      count,
			"offset":     0,
			"totalCount": count,
		},
		"filterableFields": []string{},
	}

	rc.RequestResponder.RespondWithOk(subApiEs, meta)
}

type AssocF func(parentId string, childIds []string) error

func UpdateAssociationsFor(ae *env.AppEnv, rc *response.RequestContext, idType response.IdType, store persistence.Store, action model.AssociationAction, field string) {
	UpdateAssociations(ae, rc, idType, func(parentId string, childIds []string) error {
		return ae.Handlers.Associations.UpdateAssociations(store, action, field, parentId, childIds...)
	})
}

func UpdateAssociations(ae *env.AppEnv, rc *response.RequestContext, idType response.IdType, assocF AssocF) {
	parentId, err := rc.GetIdFromRequest(idType)

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not find parentId property: %v", response.IdPropertyName)
		log.WithField("property", response.IdPropertyName).
			Error(logErr)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	body, err := ioutil.ReadAll(rc.Request.Body)

	in := &associationIdArrayRequest{}

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
	}

	il := gojsonschema.NewBytesLoader(body)

	result, err := ae.Schemes.Association.Put.Validate(il)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	if !result.Valid() {
		var errs []*apierror.ValidationError
		for _, re := range result.Errors() {
			errs = append(errs, apierror.NewValidationError(re))
		}
		rc.RequestResponder.RespondWithValidationErrors(errs)
		return
	}

	err = unmarshal(body, in)

	if err != nil {
		rc.RequestResponder.RespondWithCouldNotParseBody(err)
		return
	}

	for i, cid := range in.Ids {
		_, err := uuid.Parse(cid)

		if err != nil {
			fieldErr := apierror.NewFieldError(fmt.Sprintf("invalid UUID as ID [%s]: %s", cid, err), fmt.Sprintf("ids[%d]", i), cid)
			rc.RequestResponder.RespondWithApiError(apierror.NewField(fieldErr))
			return
		}
	}

	err = assocF(parentId, in.Ids)

	if err != nil {
		if util.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		if fe, ok := err.(*model.FieldError); ok {
			rc.RequestResponder.RespondWithFieldError(fe)
			return
		}

		rc.RequestResponder.RespondWithError(err)
		return
	}

	rc.RequestResponder.RespondWithOk(nil, nil)
}

func RemoveAssociationFor(ae *env.AppEnv, rc *response.RequestContext, idType response.IdType, store persistence.Store, field string) {
	RemoveAssociationForModel(rc, idType, func(parentId string, childIds []string) error {
		return ae.Handlers.Associations.UpdateAssociations(store, model.AssociationsActionRemove, field, parentId, childIds...)
	})
}

func RemoveAssociationForModel(rc *response.RequestContext, idType response.IdType, assocF AssocF) {
	parentId, err := rc.GetIdFromRequest(idType)

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not load parent id property: %v", response.IdPropertyName)
		log.WithField("property", response.IdPropertyName).
			Error(logErr)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	subId, err := rc.GetSubIdFromRequest()

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not find assigned entity property: %v", response.SubIdPropertyName)
		log.WithField("property", response.SubIdPropertyName).
			Error(logErr)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	err = assocF(parentId, []string{subId})

	if err != nil {
		if util.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		if fe, ok := err.(*model.FieldError); ok {
			rc.RequestResponder.RespondWithFieldError(fe)
			return
		}

		log := pfxlog.Logger()
		log.WithField("parentId", parentId).
			WithField("cause", err).
			Errorf("could not load parent record by id [%s]: %s", parentId, err)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	rc.RequestResponder.RespondWithOk(nil, nil)
}
