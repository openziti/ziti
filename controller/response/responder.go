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

package response

import (
	"fmt"
	"github.com/go-openapi/errors"
	"github.com/openziti/edge/internal/version"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/foundation/util/errorz"
	"net/http"
)

//todo: rename to Responder, remove old Responder and RequestResponder
type Responder interface {
	api.Responder
	RespondWithOk(data interface{}, meta *rest_model.Meta)
	RespondWithCreatedId(id string, link rest_model.Link)
}

type EdgeResponseMapper struct{}

func (EdgeResponseMapper) EmptyOkData() interface{} {
	return &rest_model.Empty{
		Data: map[string]interface{}{},
		Meta: &rest_model.Meta{},
	}
}

func (self EdgeResponseMapper) MapApiError(requestId string, apiError *errorz.ApiError) interface{} {
	return &rest_model.APIErrorEnvelope{
		Error: self.toRestModel(apiError, requestId),
		Meta: &rest_model.Meta{
			APIEnrollmentVersion: version.GetApiEnrollmentVersion(),
			APIVersion:           version.GetApiVersion(),
		},
	}
}

func (self EdgeResponseMapper) toRestModel(e *errorz.ApiError, requestId string) *rest_model.APIError {
	ret := &rest_model.APIError{
		Args:      nil,
		Code:      e.Code,
		Message:   e.Message,
		RequestID: requestId,
	}

	if e.Cause != nil {

		//unwrap first error in composite error
		compositeErr, ok := e.Cause.(*errors.CompositeError)
		for ok {
			e.Cause = compositeErr.Errors[0]
			compositeErr, ok = e.Cause.(*errors.CompositeError)
		}

		if causeApiError, ok := e.Cause.(*errorz.ApiError); ok {
			//standard apierror
			ret.Cause = &rest_model.APIErrorCause{
				APIError: *self.toRestModel(causeApiError, requestId),
			}
		} else if causeJsonSchemaError, ok := e.Cause.(*apierror.ValidationErrors); ok {
			//only possible from config type JSON schema validation
			ret.Cause = &rest_model.APIErrorCause{
				APIFieldError: rest_model.APIFieldError{
					Field:  causeJsonSchemaError.Errors[0].Field,
					Reason: causeJsonSchemaError.Errors[0].Error(),
					Value:  fmt.Sprintf("%v", causeJsonSchemaError.Errors[0].Value),
				},
			}
		} else if causeFieldErr, ok := e.Cause.(*errorz.FieldError); ok {
			//authenticator modules and enrollment only
			//todo: see if we can remove this by not using FieldError
			ret.Cause = &rest_model.APIErrorCause{
				APIFieldError: rest_model.APIFieldError{
					Field:  causeFieldErr.FieldName,
					Value:  fmt.Sprintf("%v", causeFieldErr.FieldValue),
					Reason: causeFieldErr.Reason,
				},
			}
			if ret.Code == errorz.InvalidFieldCode {
				ret.Code = errorz.CouldNotValidateCode
				ret.Message = errorz.CouldNotValidateMessage
			}

		} else if causeFieldErr, ok := e.Cause.(*errors.Validation); ok {
			//open api validation errors
			ret.Cause = &rest_model.APIErrorCause{
				APIFieldError: rest_model.APIFieldError{
					Field:  causeFieldErr.Name,
					Reason: causeFieldErr.Error(),
					Value:  fmt.Sprintf("%v", causeFieldErr.Value),
				},
			}
			ret.Code = errorz.CouldNotValidateCode
			ret.Message = errorz.CouldNotValidateMessage

		} else if genericErr, ok := e.Cause.(apierror.GenericCauseError); ok {
			ret.Cause = &rest_model.APIErrorCause{
				APIError: rest_model.APIError{
					Data:    genericErr.DataMap,
					Message: genericErr.Error(),
				},
			}
		} else {
			ret.Cause = &rest_model.APIErrorCause{
				APIError: rest_model.APIError{
					Code:    errorz.UnhandledCode,
					Message: e.Cause.Error(),
				},
			}
		}

	}

	return ret
}

func NewResponder(rc *RequestContext) *ResponderImpl {
	return &ResponderImpl{
		Responder: api.NewResponder(rc, EdgeResponseMapper{}),
	}
}

type ResponderImpl struct {
	api.Responder
}

func (responder *ResponderImpl) RespondWithCreatedId(id string, link rest_model.Link) {
	createEnvelope := &rest_model.CreateEnvelope{
		Data: &rest_model.CreateLocation{
			Links: rest_model.Links{
				"self": link,
			},
			ID: id,
		},
		Meta: &rest_model.Meta{},
	}

	responder.Respond(createEnvelope, http.StatusCreated)
}

func (responder *ResponderImpl) RespondWithOk(data interface{}, meta *rest_model.Meta) {
	responder.Respond(&rest_model.Empty{
		Data: data,
		Meta: meta,
	}, http.StatusOK)
}
