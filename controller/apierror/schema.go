package apierror

import (
	"encoding/json"
	"fmt"
	"github.com/xeipuuv/gojsonschema"
)

type ValidationErrors struct {
	Errors []*ValidationError
}

func (e ValidationErrors) Error() string {
	return "schema validation failed"
}

func (e ValidationErrors) MarshalJSON() ([]byte, error) {
	if len(e.Errors) > 1 {
		errMap := map[string]interface{}{
			"Reason": "multiple validation errors occurred",
			"Errors": e.Errors,
		}
		return json.Marshal(errMap)
	}

	if len(e.Errors) == 1 {
		return json.Marshal(e.Errors[0])
	}

	return nil, nil
}

type ValidationError struct {
	Field   string                 `json:"field"`
	Type    string                 `json:"type"`
	Value   interface{}            `json:"value"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s is invalid: %s", e.Field, e.Message)
}

func NewValidationError(err gojsonschema.ResultError) *ValidationError {
	return &ValidationError{
		Field:   err.Field(),
		Type:    err.Type(),
		Value:   err.Value(),
		Message: err.String(),
		Details: err.Details(),
	}
}

func NewValidationErrors(result *gojsonschema.Result) *ValidationErrors {
	var errs []*ValidationError
	for _, re := range result.Errors() {
		errs = append(errs, NewValidationError(re))
	}
	return &ValidationErrors{Errors: errs}
}
