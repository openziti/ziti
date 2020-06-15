package env

import (
	"errors"
	"fmt"
	openApiErrors "github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/google/uuid"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/response"
	"net/http"
)

// Wrapper for the OpenAPI REST server to allow the the Edge API Error message responses to be used
func ServeError(rw http.ResponseWriter, r *http.Request, inErr error) {
	if openApiError, ok := inErr.(openApiErrors.Error); ok {
		//openApiErrors from the Open API framework mean that we never hit any of the Edge logic and thus
		//do not have any context established (i.e. no request id)
		var apiError *apierror.ApiError
		if openApiError.Code() == http.StatusUnprocessableEntity {
			// triggered by validation failures and consumer errors
			var newApiError *apierror.ApiError

			if compositeError, ok := openApiError.(*openApiErrors.CompositeError); ok {
				if len(compositeError.Errors) > 0 {
					//validation errors
					if validationError, ok := compositeError.Errors[0].(*openApiErrors.Validation); ok {
						newApiError = apierror.NewCouldNotValidate(validationError)
					}
				}
			}

			// only other option is could not parse
			if newApiError == nil {
				newApiError = apierror.NewCouldNotParseBody(openApiError)
			}

			apiError = newApiError

		} else if openApiError.Code() == http.StatusNotFound {
			// handle open API openApiErrors we have existing ApiErrors for
			apiError = apierror.NewNotFound()
		} else if openApiError.Code() == http.StatusMethodNotAllowed {
			apiError = apierror.NewMethodNotAllowed()
		} else if openApiError.Code() == http.StatusUnauthorized {
			apiError = apierror.NewUnauthorized()
		} else if openApiError.Code() >= 600 && openApiError.Code() < 700 {
			//openapi defines error codes 601+ for validation errors
			apiError = apierror.NewCouldNotValidate(inErr)

		} else {
			apiError = apierror.NewUnhandled(openApiError)
		}
		apiError.Cause = openApiError

		response.RespondWithApiError(rw, uuid.New(), runtime.JSONProducer(), apiError)
		return
	}

	requestContext, err := GetRequestContextFromHttpContext(r)

	if err != nil {
		apiError := apierror.NewUnhandled(err)

		apiError.Cause = fmt.Errorf("error retrieveing request context: %w", err)
		requestId := uuid.New()
		response.RespondWithApiError(rw, requestId, runtime.JSONProducer(), apiError)
		return
	}

	if requestContext == nil {
		apiError := apierror.NewUnhandled(err)
		apiError.Cause = errors.New("expected request context is nil")
		requestId := uuid.New()
		response.RespondWithApiError(rw, requestId, runtime.JSONProducer(), apiError)
		return
	}

	requestContext.RespondWithError(inErr)
}
