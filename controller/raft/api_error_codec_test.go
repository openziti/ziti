package raft

import (
	"errors"
	"testing"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

func roundTrip(t *testing.T, apiErr *errorz.ApiError) *errorz.ApiError {
	data, err := EncodeApiError(apiErr)
	require.NoError(t, err)
	decoded := DecodeApiError(data)
	var result *errorz.ApiError
	require.True(t, errors.As(decoded, &result), "decoded value is an API error: %v", decoded)
	require.Equal(t, apiErr.AppCode, result.AppCode)
	require.Equal(t, apiErr.Status, result.Status)
	require.Equal(t, apiErr.Message, result.Message)
	return result
}

func TestApiErrorCodec_RoundTrip(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		apiErr := errorz.NewNotFound()
		apiErr.Cause = boltz.NewNotFoundError("service", "id", "s1")
		result := roundTrip(t, apiErr)
		var cause *boltz.RecordNotFoundError
		require.True(t, errors.As(result.Cause, &cause))
		require.Equal(t, "service", cause.EntityType)
		require.Equal(t, "s1", cause.Id)
	})

	t.Run("field error", func(t *testing.T) {
		result := roundTrip(t, errorz.NewFieldApiError(errorz.NewFieldError("must be unique", "name", "svc")))
		var cause *errorz.FieldError
		require.True(t, errors.As(result.Cause, &cause))
		require.Equal(t, "name", cause.FieldName)
		require.Equal(t, "svc", cause.FieldValue)
		require.Equal(t, "must be unique", cause.Reason)
	})

	t.Run("referenced entity", func(t *testing.T) {
		result := roundTrip(t, apierror.NewCanNotDeleteReferencedEntity("service", "servicePolicy", []string{"p1", "p2"}, "services"))
		var cause *errorz.FieldError
		require.True(t, errors.As(result.Cause, &cause))
		require.Equal(t, "services", cause.FieldName)
	})

	t.Run("unique index duplicate as a raw cause", func(t *testing.T) {
		apiErr := errorz.NewUnhandled(&boltz.UniqueIndexDuplicateError{Field: "name", Value: "svc", EntityType: "service"})
		result := roundTrip(t, apiErr)
		var cause *boltz.UniqueIndexDuplicateError
		require.True(t, errors.As(result.Cause, &cause))
		require.Equal(t, "name", cause.Field)
		require.Equal(t, "svc", cause.Value)
	})

	t.Run("reference exists as a raw cause", func(t *testing.T) {
		apiErr := errorz.NewUnhandled(&boltz.ReferenceExistsError{LocalType: "service", RemoteType: "servicePolicy", RemoteField: "services", LocalId: "s1", RemoteIds: []string{"p1", "p2"}})
		result := roundTrip(t, apiErr)
		var cause *boltz.ReferenceExistsError
		require.True(t, errors.As(result.Cause, &cause))
		require.Equal(t, []string{"p1", "p2"}, cause.RemoteIds)
		require.Equal(t, "s1", cause.LocalId)
	})

	t.Run("single validation error", func(t *testing.T) {
		ve := &apierror.ValidationErrors{Errors: []*apierror.ValidationError{{Field: "data.port", Type: "integer", Value: "x", Message: "not an integer"}}}
		result := roundTrip(t, errorz.NewCouldNotValidate(ve))
		var cause *apierror.ValidationErrors
		require.True(t, errors.As(result.Cause, &cause))
		require.Len(t, cause.Errors, 1)
		require.Equal(t, "data.port", cause.Errors[0].Field)
		require.Equal(t, "not an integer", cause.Errors[0].Message)
	})

	t.Run("multiple validation errors", func(t *testing.T) {
		ve := &apierror.ValidationErrors{Errors: []*apierror.ValidationError{{Field: "a", Message: "first"}, {Field: "b", Message: "second", Details: map[string]any{"k": "v"}}}}
		result := roundTrip(t, errorz.NewCouldNotValidate(ve))
		var cause *apierror.ValidationErrors
		require.True(t, errors.As(result.Cause, &cause))
		require.Len(t, cause.Errors, 2)
		require.Equal(t, "b", cause.Errors[1].Field)
		require.Equal(t, "v", cause.Errors[1].Details["k"])
	})

	t.Run("unregistered cause keeps its message", func(t *testing.T) {
		result := roundTrip(t, errorz.NewUnhandled(errors.New("something local")))
		require.NotNil(t, result.Cause)
		require.Equal(t, "something local", result.Cause.Error())
	})

	t.Run("no cause", func(t *testing.T) {
		result := roundTrip(t, errorz.NewNotFound())
		require.Nil(t, result.Cause)
	})
}
