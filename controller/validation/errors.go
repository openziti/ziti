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

import "fmt"

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
