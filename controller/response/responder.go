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

package response

import (
	"github.com/gorilla/mux"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/validation"
	"net/http"
)

type RequestResponder interface {
	RespondWithFieldError(err *validation.FieldError)
	RespondWithApiError(apiError *apierror.ApiError)
	RespondWithCouldNotParseBody(e error)
	RespondWithCouldNotReadBody(e error)
	RespondWithCreatedId(id string, link *Link)
	RespondWithCreated(data interface{}, meta *Meta, link *Link)
	RespondWithError(e error)
	RespondWithNotFound()
	RespondWithOk(data interface{}, meta *Meta)
	RespondWithUnauthorizedError(rc *RequestContext)
	RespondWithValidationErrors(ves []*apierror.ValidationError)
}

type RequestResponderFactory func(rc *RequestContext) RequestResponder

type RequestResponderImpl struct {
	rc *RequestContext
}

func NewRequestResponder(rc *RequestContext) RequestResponder {
	return &RequestResponderImpl{
		rc: rc,
	}
}

func (rr *RequestResponderImpl) RespondWithApiError(apiError *apierror.ApiError) {
	log := pfxlog.Logger()

	urlVars := mux.Vars(rr.rc.Request)
	args := map[string]interface{}{
		"urlVars": urlVars,
	}

	rsp, err := NewErrorResponse(apiError, args)

	if err != nil {
		log.WithField("cause", err).Error("could not create responder")
		return
	}

	err = rsp.Respond(rr.rc)

	if err != nil {
		log.WithField("cause", err).Error("could not respond")
		return
	}
}

func (rr *RequestResponderImpl) RespondWithFieldError(err *validation.FieldError) {
	rr.RespondWithApiError(apierror.NewField(apierror.NewFieldError(err.Reason, err.FieldName, err.FieldValue)))
}

func (rr *RequestResponderImpl) RespondWithCouldNotParseBody(e error) {
	rr.RespondWithApiError(&apierror.ApiError{
		Code:    apierror.CouldNotParseBodyCode,
		Message: apierror.CouldNotParseBodyMessage,
		Cause:   e,
		Status:  http.StatusBadRequest,
	})
}

func (rr *RequestResponderImpl) RespondWithCouldNotReadBody(e error) {
	rr.RespondWithApiError(&apierror.ApiError{
		Code:    apierror.CouldNotReadBodyCode,
		Message: apierror.CouldNotReadBodyMessage,
		Cause:   e,
		Status:  http.StatusBadRequest,
	})
}

func (rr *RequestResponderImpl) RespondWithCreatedId(id string, link *Link) {
	RespondWithSimpleCreated(id, link, rr.rc)
}

func (rr *RequestResponderImpl) RespondWithCreated(data interface{}, meta *Meta, link *Link) {
	RespondWithCreated(data, meta, link, rr.rc)
}

func (rr *RequestResponderImpl) RespondWithError(e error) {
	apiErr, ok := e.(*apierror.ApiError)

	if ok {
		rr.RespondWithApiError(apiErr)
		return
	}

	log := pfxlog.Logger()
	log.WithField("cause", e).Errorf("unhandled error: %+v", e)
	vars := mux.Vars(rr.rc.Request)

	args := map[string]interface{}{
		"cause":   e,
		"urlVars": vars,
	}

	rsp, err := NewErrorResponse(&apierror.ApiError{
		Code:    apierror.UnhandledCode,
		Message: apierror.UnhandledMessage,
		Status:  http.StatusInternalServerError,
		Cause:   e,
	}, args)

	if err != nil {
		log.WithField("cause", err).Error("could not create responder")
		return
	}

	err = rsp.Respond(rr.rc)

	if err != nil {
		log.WithField("cause", err).Error("could not respond")
		return
	}
}

func (rr *RequestResponderImpl) RespondWithNotFound() {
	rr.RespondWithApiError(&apierror.ApiError{
		Code:    apierror.NotFoundCode,
		Message: apierror.NotFoundMessage,
		Status:  http.StatusNotFound,
	})
}

func (rr *RequestResponderImpl) RespondWithOk(data interface{}, meta *Meta) {
	RespondWithOk(data, meta, rr.rc)
}

func (rr *RequestResponderImpl) RespondWithUnauthorizedError(rc *RequestContext) {
	rr.RespondWithApiError(apierror.NewUnauthorized())
}

func (rr *RequestResponderImpl) RespondWithValidationErrors(ves []*apierror.ValidationError) {
	rr.RespondWithApiError(&apierror.ApiError{
		Code:    apierror.CouldNotValidateCode,
		Message: apierror.CouldNotValidateMessage,
		Cause:   ves[0],
		Status:  http.StatusBadRequest,
	})
}
