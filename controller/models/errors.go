package models

import (
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/boltz"
)

func ToApiError(err error) *errorz.ApiError {
	if apiErr, ok := err.(*errorz.ApiError); ok {
		return apiErr
	}

	if boltz.IsErrNotFoundErr(err) {
		result := errorz.NewNotFound()
		result.Cause = err
		return result
	}

	if fe, ok := err.(*errorz.FieldError); ok {
		return errorz.NewFieldApiError(fe)
	}

	if sve, ok := err.(*apierror.ValidationErrors); ok {
		return errorz.NewCouldNotValidate(sve)
	}

	return errorz.NewUnhandled(err)
}
