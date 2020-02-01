/*
	Copyright 2019 Netfoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package validation

import (
	"fmt"
	"github.com/xeipuuv/gojsonschema"
)

const (
	maxFieldErrorValueLength = 20
)

type FieldError struct {
	Reason     string
	FieldName  string
	FieldValue interface{}
}

func (fe FieldError) Error() string {
	v := fe.FieldValue
	if s, ok := fe.FieldValue.(string); ok {
		if len(s) > maxFieldErrorValueLength {
			v = s[0:maxFieldErrorValueLength] + "..."
		}
	}
	return fmt.Sprintf("the value '%v' for '%s' is invalid: %s", v, fe.FieldName, fe.Reason)
}

func NewFieldError(reason, name string, value interface{}) *FieldError {
	return &FieldError{
		Reason:     reason,
		FieldName:  name,
		FieldValue: value,
	}
}

type SchemaValidationErrors struct {
	Errors []*SchemaValidationError
}

func (e SchemaValidationErrors) Error() string {
	return fmt.Sprintf("schema validation failed")
}

type SchemaValidationError struct {
	Field   string                 `json:"field"`
	Type    string                 `json:"type"`
	Value   interface{}            `json:"value"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details"`
}

func (e SchemaValidationError) Error() string {
	return fmt.Sprintf("%s is invalid: %s", e.Field, e.Message)
}

func NewValidationError(err gojsonschema.ResultError) *SchemaValidationError {
	return &SchemaValidationError{
		Field:   err.Field(),
		Type:    err.Type(),
		Value:   err.Value(),
		Message: err.String(),
		Details: err.Details(),
	}
}

func NewSchemaValidationErrors(result *gojsonschema.Result) *SchemaValidationErrors {
	var errs []*SchemaValidationError
	for _, re := range result.Errors() {
		errs = append(errs, NewValidationError(re))
	}
	return &SchemaValidationErrors{Errors: errs}
}
