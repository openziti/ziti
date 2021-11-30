package api

import (
	"github.com/go-openapi/runtime"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/foundation/util/errorz"
	"net/http"
	"strings"
)

func NewResponder(rc RequestContext, mapper ResponseMapper) *ResponderImpl {
	return &ResponderImpl{
		rc:       rc,
		mapper:   mapper,
		producer: runtime.JSONProducer(),
	}
}

type ResponderImpl struct {
	rc       RequestContext
	mapper   ResponseMapper
	producer runtime.Producer
}

func (responder *ResponderImpl) SetProducer(producer runtime.Producer) {
	responder.producer = producer
}

func (responder *ResponderImpl) GetProducer() runtime.Producer {
	return responder.producer
}

func (responder *ResponderImpl) RespondWithCouldNotReadBody(err error) {
	responder.RespondWithApiError(apierror.NewCouldNotReadBody(err))
}

func (responder *ResponderImpl) RespondWithCouldNotParseBody(err error) {
	responder.RespondWithApiError(apierror.NewCouldNotParseBody(err))
}

func (responder *ResponderImpl) RespondWithValidationErrors(errors *apierror.ValidationErrors) {
	responder.RespondWithApiError(errorz.NewCouldNotValidate(errors))
}

func (responder *ResponderImpl) RespondWithNotFound() {
	responder.RespondWithApiError(errorz.NewNotFound())
}

func (responder *ResponderImpl) RespondWithNotFoundWithCause(cause error) {
	apiErr := errorz.NewNotFound()
	apiErr.Cause = cause
	responder.RespondWithApiError(apiErr)
}

func (responder *ResponderImpl) RespondWithFieldError(fe *errorz.FieldError) {
	responder.RespondWithApiError(errorz.NewFieldApiError(errorz.NewFieldError(fe.Reason, fe.FieldName, fe.FieldValue)))
}

func (responder *ResponderImpl) RespondWithEmptyOk() {
	responder.Respond(responder.mapper.EmptyOkData(), http.StatusOK)
}

func (responder *ResponderImpl) Respond(data interface{}, httpStatus int) {
	responder.RespondWithProducer(responder.GetProducer(), data, httpStatus)
}

func (responder *ResponderImpl) RespondWithProducer(producer runtime.Producer, data interface{}, httpStatus int) {
	w := responder.rc.GetResponseWriter()
	w.WriteHeader(httpStatus)

	err := producer.Produce(w, data)

	if err != nil {
		pfxlog.Logger().WithError(err).
			WithField("requestId", responder.rc.GetId()).
			WithField("path", responder.rc.GetRequest().URL.Path).
			Debug("could not respond, producer errored, possible timeout")
	}
}

func (responder *ResponderImpl) RespondWithError(err error) {
	var apiError *errorz.ApiError
	var ok bool

	if apiError, ok = err.(*errorz.ApiError); !ok {
		apiError = errorz.NewUnhandled(err)
	}

	responder.RespondWithApiError(apiError)
}

func (responder *ResponderImpl) RespondWithApiError(apiError *errorz.ApiError) {
	data := responder.mapper.MapApiError(responder.rc.GetId(), apiError)

	producer := responder.rc.GetProducer()
	w := responder.rc.GetResponseWriter()

	if canRespondWithJson(responder.rc.GetRequest()) {
		producer = runtime.JSONProducer()
		w.Header().Set("content-type", "application/json")
	}

	w.WriteHeader(apiError.Status)
	err := producer.Produce(w, data)

	if err != nil {
		pfxlog.Logger().WithError(err).WithField("requestId", responder.rc.GetId()).Error("could not respond with error, producer errored")
	}
}

func canRespondWithJson(request *http.Request) bool {
	//if we can return JSON for errors we should as they provide the most
	//information

	canReturnJson := false

	acceptHeaders := request.Header.Values("accept")
	if len(acceptHeaders) == 0 {
		//no accept == "*/*"
		canReturnJson = true
	} else {
		for _, acceptHeader := range acceptHeaders { //look at all headers values
			if canReturnJson {
				break
			}

			for _, value := range strings.Split(acceptHeader, ",") { //each header can have multiple mimeTypes
				mimeType := strings.Split(value, ";")[0] //remove quotients
				mimeType = strings.TrimSpace(mimeType)

				if mimeType == "*/*" || mimeType == "application/json" {
					canReturnJson = true
					break
				}
			}
		}
	}
	return canReturnJson
}
