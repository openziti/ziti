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

package apierror

import "fmt"

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
