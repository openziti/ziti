/*
	Copyright NetFoundry, Inc.

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

package apierror

import (
	"encoding/json"
	"fmt"
	"strings"
)

type BodyParseTypeError struct {
	Near            string `json:"near"`
	ExpectedType    string `json:"expectedType"`
	EncounteredType string `json:"encounteredType"`
	Offset          int64  `json:"totalOffset"`
	Line            int64  `json:"line"`
}

func (e BodyParseTypeError) Error() string {
	return fmt.Sprintf("Expected type %s, but encountered %s near %s", e.ExpectedType, e.ExpectedType, e.Near)
}

func NewBodyParseTypeError(e *json.UnmarshalTypeError, body string) *BodyParseTypeError {
	nearStart := e.Offset - 10
	nearEnd := e.Offset

	if nearStart < 0 {
		nearStart = 0
	}

	near := body[nearStart:nearEnd]

	bodyToError := body[:e.Offset]

	line := strings.Count(bodyToError, "\n") + 1

	return &BodyParseTypeError{
		ExpectedType:    e.Type.String(),
		EncounteredType: e.Value,
		Near:            near,
		Offset:          e.Offset,
		Line:            int64(line),
	}
}

type BodyParseSyntaxError struct {
	Near   string `json:"near"`
	Offset int64  `json:"totalOffset"`
	Line   int64  `json:"line"`
}

func (e BodyParseSyntaxError) Error() string {
	return fmt.Sprintf("Invalid syntax on line %d", e.Line)
}

func NewBodyParseSyntaxError(e *json.SyntaxError, body string) *BodyParseSyntaxError {
	nearStart := e.Offset - 10
	nearEnd := e.Offset

	if nearStart < 0 {
		nearStart = 0
		nearEnd = 5
	}

	if nearEnd > int64(len(body)) {
		nearEnd = int64(len(body))
	}

	near := body[nearStart:nearEnd]

	bodyToError := body[:e.Offset]

	line := strings.Count(bodyToError, "\n") + 1

	return &BodyParseSyntaxError{
		Near:   near,
		Offset: e.Offset,
		Line:   int64(line),
	}
}
