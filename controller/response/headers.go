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

package response

import (
	"github.com/openziti/fabric/build"
	"strconv"
)

const (
	ApiSessionExpirationSecondsHeader = "expiration-seconds"
	ApiSessionExpiresAtHeader         = "expires-at"
	ServerHeader                      = "server"
)

func AddHeaders(rc *RequestContext) {
	buildInfo := build.GetBuildInfo()
	if buildInfo != nil {
		rc.ResponseWriter.Header().Set(ServerHeader, "ziti-controller/"+buildInfo.Version())
	}

	AddSessionHeaders(rc)
}

func AddSessionHeaders(rc *RequestContext) {
	if rc.ApiSession != nil {
		rc.ResponseWriter.Header().Set(ApiSessionExpirationSecondsHeader, strconv.FormatInt(int64(rc.ApiSession.ExpirationDuration.Seconds()), 10))
		rc.ResponseWriter.Header().Set(ApiSessionExpiresAtHeader, rc.ApiSession.ExpiresAt.String())
	}
}
