/*
	Copyright NetFoundry Inc.

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

package raft

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/michaelquigley/pfxlog"
	"github.com/mitchellh/mapstructure"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
)

// causeParser rebuilds a cause from the JSON object EncodeApiError sent for it. The first result is
// the cause; the second reports a malformed object.
type causeParser func(m map[string]any) (error, error)

// causeParsers maps a cause's Go type name to its parser. The encoder consults the same table, so a
// cause type is either carried structurally on both sides or reduced to its message on both.
var causeParsers = map[string]causeParser{
	causeTypeName(&boltz.RecordNotFoundError{}):       parseStruct[boltz.RecordNotFoundError],
	causeTypeName(&boltz.UniqueIndexDuplicateError{}): parseStruct[boltz.UniqueIndexDuplicateError],
	causeTypeName(&boltz.ReferenceExistsError{}):      parseStruct[boltz.ReferenceExistsError],
	causeTypeName(&errorz.FieldError{}):               parseFieldError,
	causeTypeName(&apierror.ValidationErrors{}):       parseValidationErrors,
}

func causeTypeName(cause error) string {
	return fmt.Sprintf("%T", cause)
}

// EncodeApiError serializes apiErr for the peer channel. A cause whose type has a parser crosses as
// a JSON object and is rebuilt by DecodeApiError; any other cause crosses as its message.
func EncodeApiError(apiErr *errorz.ApiError) ([]byte, error) {
	m := map[string]any{
		"code":    apiErr.AppCode,
		"message": apiErr.Message,
		"status":  apiErr.Status,
	}

	if apiErr.Cause != nil {
		typeName := causeTypeName(apiErr.Cause)
		if _, structured := causeParsers[typeName]; structured {
			if causeBytes, err := json.Marshal(apiErr.Cause); err == nil && string(causeBytes) != "{}" {
				m["causeType"] = typeName
				m["cause"] = json.RawMessage(causeBytes)
			} else {
				m["cause"] = apiErr.Cause.Error()
			}
		} else {
			m["cause"] = apiErr.Cause.Error()
		}
	}

	return json.Marshal(m)
}

// DecodeApiError rebuilds an API error encoded by EncodeApiError. Input that is not such an
// encoding comes back as a plain error carrying the raw text, so a caller always gets something
// it can return.
func DecodeApiError(data []byte) error {
	m := map[string]any{}
	if err := json.Unmarshal(data, &m); err != nil {
		pfxlog.Logger().Warnf("invalid api error encoding, unable to decode: %v", string(data))
		return errors.New(string(data))
	}

	apiErr := &errorz.ApiError{}

	code, ok := m["code"].(string)
	if !ok {
		pfxlog.Logger().Warnf("invalid api error encoding, code missing or not a string: %v", string(data))
		return errors.New(string(data))
	}
	apiErr.AppCode = code

	status, ok := m["status"]
	if !ok {
		pfxlog.Logger().Warnf("invalid api error encoding, no status: %v", string(data))
		return errors.New(string(data))
	}
	statusInt, err := strconv.Atoi(fmt.Sprintf("%v", status))
	if err != nil {
		pfxlog.Logger().Warnf("invalid api error encoding, status not an int: %v", string(data))
		return errors.New(string(data))
	}
	apiErr.Status = statusInt

	message, ok := m["message"].(string)
	if !ok {
		pfxlog.Logger().Warnf("invalid api error encoding, message missing or not a string: %v", string(data))
		return errors.New(string(data))
	}
	apiErr.Message = message

	if cause, ok := m["cause"]; ok && cause != nil {
		apiErr.Cause = decodeCause(m, cause)
	}

	return apiErr
}

func decodeCause(m map[string]any, cause any) error {
	if text, ok := cause.(string); ok {
		return errors.New(text)
	}

	obj, ok := cause.(map[string]any)
	if !ok {
		return fmt.Errorf("%v", cause)
	}

	if typeName, ok := m["causeType"].(string); ok {
		if parser := causeParsers[typeName]; parser != nil {
			if result, err := parser(obj); err == nil {
				return result
			} else {
				pfxlog.Logger().WithError(err).Warnf("unable to parse api error cause of type %v", typeName)
			}
		}
	}

	if b, err := json.Marshal(obj); err == nil {
		return errors.New(string(b))
	}
	return fmt.Errorf("%+v", obj)
}

// parseStruct rebuilds a cause whose exported fields marshal as they are.
func parseStruct[T any, PT interface {
	*T
	error
}](m map[string]any) (error, error) {
	result := PT(new(T))
	if err := mapstructure.Decode(m, result); err != nil {
		return nil, err
	}
	return result, nil
}

func parseFieldError(m map[string]any) (error, error) {
	field, ok := m["field"].(string)
	if !ok {
		return nil, errors.New("field error has no field name")
	}
	fieldError := &errorz.FieldError{
		FieldName:  field,
		FieldValue: m["value"],
	}
	if reason, ok := m["message"].(string); ok {
		fieldError.Reason = reason
	} else if reason, ok := m["reason"].(string); ok {
		fieldError.Reason = reason
	}
	return fieldError, nil
}

// parseValidationErrors accepts both shapes ValidationErrors.MarshalJSON emits: one error bare,
// or several under Reason and Errors.
func parseValidationErrors(m map[string]any) (error, error) {
	result := &apierror.ValidationErrors{}

	entries, multiple := m["Errors"].([]any)
	if !multiple {
		entries = []any{m}
	}
	for _, entry := range entries {
		ve := &apierror.ValidationError{}
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{TagName: "json", Result: ve})
		if err != nil {
			return nil, err
		}
		if err = decoder.Decode(entry); err != nil {
			return nil, err
		}
		result.Errors = append(result.Errors, ve)
	}
	return result, nil
}
