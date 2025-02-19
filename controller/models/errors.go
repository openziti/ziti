package models

import (
	"errors"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/apierror"
)

func ToApiError(err error) *errorz.ApiError {
	return ToApiErrorWithDefault(err, errorz.NewUnhandled)
}

func ToApiErrorWithDefault(err error, f func(err error) *errorz.ApiError) *errorz.ApiError {
	var apiErr *errorz.ApiError
	if errors.As(err, &apiErr) {
		return apiErr
	}

	if boltz.IsErrNotFoundErr(err) {
		result := errorz.NewNotFound()
		result.Cause = err
		return result
	}

	var fe *errorz.FieldError
	if errors.As(err, &fe) {
		return errorz.NewFieldApiError(fe)
	}

	var sve *apierror.ValidationErrors
	if errors.As(err, &sve) {
		return errorz.NewCouldNotValidate(sve)
	}

	return f(err)
}
