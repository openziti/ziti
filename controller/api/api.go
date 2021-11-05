package api

import (
	"github.com/go-openapi/runtime"
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/foundation/util/errorz"
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
