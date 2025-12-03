package api_impl

import (
	"fmt"

	openApiErrors "github.com/go-openapi/errors"
	"github.com/openziti/foundation/v2/errorz"
	apierror2 "github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/rest_model"
)

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

		} else if genericErr, ok := e.Cause.(*apierror2.GenericCauseError); ok {
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
