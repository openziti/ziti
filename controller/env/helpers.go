package env

import (
	openApiErrors "github.com/go-openapi/errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
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
				newApiError = apierror.NewCouldNotParseBody(openApiError)
			}

			apiError = newApiError

		} else if openApiError.Code() == http.StatusNotFound {
			// handle open API openApiErrors we have existing ApiErrors for
			apiError = errorz.NewNotFound()
		} else if openApiError.Code() == http.StatusMethodNotAllowed {
			apiError = apierror.NewMethodNotAllowed()
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

	requestContext, err := GetRequestContextFromHttpContext(r)
	if requestContext == nil || err != nil {
		pfxlog.Logger().WithError(err).Error("failed to retrieve request context")
		requestContext = NewRequestContext(rw, r)
	}

	requestContext.RespondWithError(inErr)
}
