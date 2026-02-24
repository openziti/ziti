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

package response

import (
	"errors"
	"strconv"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/build"
)

const (
	ApiSessionExpirationSecondsHeader = "expiration-seconds"
	ApiSessionExpiresAtHeader         = "expires-at"
	ServerHeader                      = "server"
)

// AddHeaders sets standard response headers for every API response, including the server
// version banner and API-session expiry information.
func AddHeaders(rc *RequestContext) {
	buildInfo := build.GetBuildInfo()
	if buildInfo != nil {
		rc.ResponseWriter.Header().Set(ServerHeader, "ziti-controller/"+buildInfo.Version())
	}

	AddApiSessionHeaders(rc)
}

// AddApiSessionHeaders writes API-session lifetime headers when a resolved session is
// available, and appends WWW-Authenticate or other structured error headers from any
// MFA or session-level errors so that clients can determine the next required action.
func AddApiSessionHeaders(rc *RequestContext) {
	if apiSession, apiSessionErr := rc.SecurityCtx.GetApiSessionWithoutResolve(); apiSession != nil {
		rc.ResponseWriter.Header().Set(ApiSessionExpirationSecondsHeader, strconv.FormatInt(int64(apiSession.ExpirationDuration.Seconds()), 10))
		rc.ResponseWriter.Header().Set(ApiSessionExpiresAtHeader, apiSession.ExpiresAt.String())

		// add any headers for MFA errors
		mfaErr := rc.SecurityCtx.GetMfaErrorWithoutResolve()

		if mfaErr != nil {
			addApiErrorHeaders(rc, mfaErr)
		}
	} else if apiSessionErr != nil {
		addApiErrorHeaders(rc, apiSessionErr)
	}
}

// addApiErrorHeaders extracts header key-value pairs from an errorz.ApiError and writes
// them to the response. This allows structured errors (e.g., WWW-Authenticate challenges)
// to propagate their metadata directly to the HTTP client.
func addApiErrorHeaders(rc *RequestContext, err error) {
	apiErr := &errorz.ApiError{}

	if errors.As(err, &apiErr) {
		for key, values := range apiErr.Headers {
			for _, value := range values {
				rc.ResponseWriter.Header().Add(key, value)
			}
		}
	}
}
