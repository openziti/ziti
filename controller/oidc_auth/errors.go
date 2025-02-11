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

package oidc_auth

import (
	"fmt"
	"github.com/openziti/foundation/v2/errorz"
)

func newNotAcceptableError(acceptHeader string) *errorz.ApiError {
	return &errorz.ApiError{
		Code:    "NOT_ACCEPTABLE",
		Message: fmt.Sprintf("the request is not acceptable, the accept header did not have any supported options: %s (supported: %s, %s)", acceptHeader, JsonContentType, HtmlContentType),
	}
}
