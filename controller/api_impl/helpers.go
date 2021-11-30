package api_impl

import (
	"fmt"
	openApiErrors "github.com/go-openapi/errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/api"
	apierror2 "github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/fabric/rest_model"
	"github.com/openziti/foundation/util/errorz"
	"net/http"
)

// Wrapper for the OpenAPI REST server to allow the the Edge API Error message responses to be used
func ServeError(rw http.ResponseWriter, r *http.Request, inErr error) {
	if openApiError, ok := inErr.(openApiErrors.Error); ok {
		//openApiErrors from the Open API framework mean that we never hit any of the Edge logic and thus
		//do not have any context established (i.e. no request id)
		var apiError *errorz.ApiError
		if openApiError.Code() == http.StatusUnprocessableEntity {
			// triggered by validation failures and consumer errors
			var newApiError *errorz.ApiError

			if compositeError, ok := openApiError.(*openApiErrors.CompositeError); ok {
				if len(compositeError.Errors) > 0 {
					//validation errors
					if validationError, ok := compositeError.Errors[0].(*openApiErrors.Validation); ok {
						newApiError = errorz.NewCouldNotValidate(validationError)
					}
				}
			}

			// only other option is could not parse
			if newApiError == nil {
				newApiError = apierror2.NewCouldNotParseBody(openApiError)
			}

			apiError = newApiError

		} else if openApiError.Code() == http.StatusNotFound {
			// handle open API openApiErrors we have existing ApiErrors for
			apiError = errorz.NewNotFound()
		} else if openApiError.Code() == http.StatusMethodNotAllowed {
			apiError = apierror2.NewMethodNotAllowed()
		} else if openApiError.Code() == http.StatusUnauthorized {
			apiError = errorz.NewUnauthorized()
		} else if openApiError.Code() >= 600 && openApiError.Code() < 700 {
			//openapi defines error codes 601+ for validation errors
			apiError = errorz.NewCouldNotValidate(inErr)

		} else {
			apiError = errorz.NewUnhandled(openApiError)
		}
		apiError.Cause = openApiError

		NewRequestContext(rw, r).RespondWithApiError(apiError)
		return
	}

	requestContext, err := api.GetRequestContextFromHttpContext(r)
	if requestContext == nil || err != nil {
		pfxlog.Logger().WithError(err).Error("failed to retrieve request context")
		requestContext = NewRequestContext(rw, r)
	}

	requestContext.RespondWithError(inErr)
}

func ToRestModel(e *errorz.ApiError, requestId string) *rest_model.APIError {
	ret := &rest_model.APIError{
		Args:      nil,
		Code:      e.Code,
		Message:   e.Message,
		RequestID: requestId,
	}

	if e.Cause != nil {

		//unwrap first error in composite error
		compositeErr, ok := e.Cause.(*openApiErrors.CompositeError)
		for ok {
			e.Cause = compositeErr.Errors[0]
			compositeErr, ok = e.Cause.(*openApiErrors.CompositeError)
		}

		if causeApiError, ok := e.Cause.(*errorz.ApiError); ok {
			//standard apierror
			ret.Cause = &rest_model.APIErrorCause{
				APIError: *ToRestModel(causeApiError, requestId),
			}
		} else if causeJsonSchemaError, ok := e.Cause.(*apierror2.ValidationErrors); ok {
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

		} else if causeFieldErr, ok := e.Cause.(*openApiErrors.Validation); ok {
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

		} else if genericErr, ok := e.Cause.(apierror2.GenericCauseError); ok {
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
