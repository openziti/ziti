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

package api

import (
	"github.com/gorilla/handlers"
	"net/http"
)

const (
	// ZitiSession is the header value used to pass Ziti sessions around
	ZitiSession = "zt-session"
)

func WrapCorsHandler(innerHandler http.Handler) http.Handler {
	corsOpts := []handlers.CORSOption{
		handlers.AllowedOrigins([]string{"*"}),
		handlers.OptionStatusCode(200),
		handlers.AllowedHeaders([]string{
			"content-type",
			"accept",
			"authorization",
			ZitiSession,
		}),
		handlers.AllowedMethods([]string{
			http.MethodGet,
			http.MethodHead,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete}),
		handlers.AllowCredentials(),
	}

	return handlers.CORS(corsOpts...)(innerHandler)
}
