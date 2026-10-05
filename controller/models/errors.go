package models

import (
	"errors"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
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

	var duplicate *boltz.UniqueIndexDuplicateError
	if errors.As(err, &duplicate) {
		return errorz.NewFieldApiError(&errorz.FieldError{
			Reason:     duplicate.Error(),
			FieldName:  duplicate.Field,
			FieldValue: duplicate.Value,
		})
	}

	var referenced *boltz.ReferenceExistsError
	if errors.As(err, &referenced) {
		return apierror.NewCanNotDeleteReferencedEntity(referenced.LocalType, referenced.RemoteType, referenced.RemoteIds, referenced.RemoteField)
	}

	return f(err)
}
