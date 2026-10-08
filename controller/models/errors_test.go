package models

import (
	"errors"
	"fmt"
	"testing"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

func TestToApiError_MapsEveryCauseClass(t *testing.T) {
	t.Run("duplicate unique index becomes a field error", func(t *testing.T) {
		req := require.New(t)
		cause := &boltz.UniqueIndexDuplicateError{Field: "name", Value: "svc", EntityType: "service"}
		apiErr := ToApiError(cause)
		req.Equal(errorz.InvalidFieldStatus, apiErr.Status)
		req.Equal(errorz.InvalidFieldCode, apiErr.AppCode)
		var fe *errorz.FieldError
		req.True(errors.As(apiErr.Cause, &fe))
		req.Equal("name", fe.FieldName)
		req.Equal("svc", fe.FieldValue)

		wrapped := ToApiError(fmt.Errorf("creating: %w", cause))
		req.Equal(errorz.InvalidFieldStatus, wrapped.Status, "a wrapped cause maps the same way")
	})

	t.Run("reference exists becomes can-not-delete-referenced-entity", func(t *testing.T) {
		req := require.New(t)
		cause := &boltz.ReferenceExistsError{LocalType: "service", RemoteType: "servicePolicy", RemoteField: "services", LocalId: "s1", RemoteIds: []string{"p1"}}
		apiErr := ToApiError(cause)
		req.Equal(apierror.CanNotDeleteReferencedEntityStatus, apiErr.Status)
		req.Equal(apierror.CanNotDeleteReferencedEntityCode, apiErr.AppCode)
		var fe *errorz.FieldError
		req.True(errors.As(apiErr.Cause, &fe))
		req.Equal("services", fe.FieldName)
	})

	t.Run("existing mappings are unchanged", func(t *testing.T) {
		req := require.New(t)
		req.Equal(errorz.NotFoundStatus, ToApiError(boltz.NewNotFoundError("service", "id", "x")).Status)
		req.Equal(errorz.InvalidFieldStatus, ToApiError(errorz.NewFieldError("bad", "name", "x")).Status)
		req.Equal(errorz.CouldNotValidateStatus, ToApiError(&apierror.ValidationErrors{Errors: []*apierror.ValidationError{{Field: "a"}}}).Status)
		req.Equal(errorz.UnhandledStatus, ToApiError(errors.New("something else")).Status)
	})
}
