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
	"github.com/go-openapi/runtime"
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/foundation/v2/errorz"
	"net/http"
)

type Responder interface {
	Respond(data interface{}, httpStatus int)
	RespondWithProducer(producer runtime.Producer, data interface{}, httpStatus int)
	RespondWithEmptyOk()
	RespondWithError(err error)
	RespondWithApiError(err *errorz.ApiError)
	SetProducer(producer runtime.Producer)
	GetProducer() runtime.Producer
	RespondWithCouldNotReadBody(err error)
	RespondWithCouldNotParseBody(err error)
	RespondWithValidationErrors(errors *apierror.ValidationErrors)
	RespondWithNotFound()
	RespondWithNotFoundWithCause(cause error)
	RespondWithFieldError(fe *errorz.FieldError)
}

type ResponseMapper interface {
	EmptyOkData() interface{}
	MapApiError(requestId string, err *errorz.ApiError) interface{}
}

type RequestContext interface {
	Responder
	GetId() string
	GetBody() []byte
	GetRequest() *http.Request
	GetResponseWriter() http.ResponseWriter
	SetEntityId(id string)
	SetEntitySubId(id string)
	GetEntityId() (string, error)
	GetEntitySubId() (string, error)
}
