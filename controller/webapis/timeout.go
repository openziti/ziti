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

package webapis

import (
	"net/http"
	"time"

	"github.com/openziti/xweb/v3"
	"github.com/openziti/ziti/v2/controller/api"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/response"
)

// apiRequestTimeout is how long a REST API request may run before it is answered with a timeout error.
const apiRequestTimeout = 10 * time.Second

// wrapWithTimeout adds CORS handling and the API request timeout to handler, keeping the connection writable for
// the timeout response regardless of serverConfig's write timeout. serverConfig may be nil.
func wrapWithTimeout(handler http.Handler, serverConfig *xweb.ServerConfig) http.Handler {
	var writeTimeout time.Duration
	if serverConfig != nil {
		writeTimeout = serverConfig.Options.WriteTimeout
	}
	return api.TimeoutHandler(api.WrapCorsHandler(handler), apiRequestTimeout, writeTimeout, apierror.NewTimeoutError(), response.EdgeResponseMapper{})
}
