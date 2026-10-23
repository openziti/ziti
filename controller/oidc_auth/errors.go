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
	"net/http"

	"github.com/openziti/foundation/v2/errorz"
)

func newUnsupportedMediaTypeError(contentType string) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: "UNSUPPORTED_MEDIA_TYPE",
		Message: fmt.Sprintf("the content type: %s, is not supported (supported: %s, %s)", contentType, FormContentType, JsonContentType),
		Status:  http.StatusUnsupportedMediaType,
	}
}

// newInvalidTotpCodeError returns the error for a wrong TOTP code. The app code keeps the
// space separated "INVALID TOTP CODE" format for legacy client support.
func newInvalidTotpCodeError() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: "INVALID TOTP CODE",
		Message: "an invalid TOTP code was supplied",
		Status:  http.StatusBadRequest,
	}
}

func newNotAcceptableError(acceptHeader string) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: "NOT_ACCEPTABLE",
		Message: fmt.Sprintf("the request is not acceptable, the accept header did not have any supported options: %s (supported: %s, %s)", acceptHeader, JsonContentType, HtmlContentType),
	}
}
