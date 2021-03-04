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
	"github.com/go-openapi/runtime"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/schema"
	"github.com/openziti/edge/internal/version"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/foundation/util/errorz"
	"net/http"
	"strings"
)

//todo: rename to Responder, remove old Responder and RequestResponder
type Responder interface {
	Respond(data interface{}, httpStatus int)
	RespondWithOk(data interface{}, meta *rest_model.Meta)
	RespondWithEmptyOk()
	RespondWithError(err error)
	RespondWithApiError(apiError *errorz.ApiError)
	SetProducer(producer runtime.Producer)
	GetProducer() runtime.Producer
	RespondWithCouldNotReadBody(err error)
	RespondWithCouldNotParseBody(err error)
	RespondWithValidationErrors(errors *schema.ValidationErrors)
	RespondWithNotFound()
	RespondWithNotFoundWithCause(cause error)
	RespondWithFieldError(fe *errorz.FieldError)
	RespondWithCreatedId(id string, link rest_model.Link)
}

func NewResponder(rc *RequestContext) *ResponderImpl {
	return &ResponderImpl{rc: rc, producer: runtime.JSONProducer()}
}

type ResponderImpl struct {
	rc       *RequestContext
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

func (responder *ResponderImpl) RespondWithValidationErrors(errors *schema.ValidationErrors) {
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

func (responder *ResponderImpl) RespondWithCreatedId(id string, link rest_model.Link) {
	createEnvelope := &rest_model.CreateEnvelope{
		Data: &rest_model.CreateLocation{
			Links: rest_model.Links{
				"self": link,
			},
			ID: id,
		},
		Meta: &rest_model.Meta{},
	}

	responder.Respond(createEnvelope, http.StatusCreated)
}

func (responder *ResponderImpl) RespondWithEmptyOk() {
	responder.Respond(&rest_model.Empty{Data: map[string]interface{}{}, Meta: &rest_model.Meta{}}, http.StatusOK)
}

func (responder *ResponderImpl) RespondWithOk(data interface{}, meta *rest_model.Meta) {
	responder.Respond(&rest_model.Empty{
		Data: data,
		Meta: meta,
	}, http.StatusOK)
}

func (responder *ResponderImpl) Respond(data interface{}, httpStatus int) {
	Respond(responder.rc.ResponseWriter, responder.rc.Request.URL.Path, responder.rc.Id, responder.GetProducer(), data, httpStatus)
}

func (responder *ResponderImpl) RespondWithError(err error) {
	RespondWithError(responder.rc.ResponseWriter, responder.rc.Request, responder.rc.Id, responder.GetProducer(), err)
}

func (responder *ResponderImpl) RespondWithApiError(apiError *errorz.ApiError) {
	RespondWithApiError(responder.rc.ResponseWriter, responder.rc.Request, responder.rc.Id, responder.GetProducer(), apiError)
}

func Respond(w http.ResponseWriter, path, requestId string, producer runtime.Producer, data interface{}, httpStatus int) {
	w.WriteHeader(httpStatus)

	err := producer.Produce(w, data)

	if err != nil {
		pfxlog.Logger().WithError(err).WithField("requestId", requestId).WithField("path", path).Debug("could not respond, producer errored, possible timeout")
	}
}

func RespondWithError(w http.ResponseWriter, r *http.Request, requestId string, producer runtime.Producer, err error) {
	var apiError *errorz.ApiError
	var ok bool

	if apiError, ok = err.(*errorz.ApiError); !ok {
		apiError = errorz.NewUnhandled(err)
	}

	RespondWithApiError(w, r, requestId, producer, apiError)
}

func RespondWithApiError(w http.ResponseWriter, r *http.Request, requestId string, producer runtime.Producer, apiError *errorz.ApiError) {
	data := &rest_model.APIErrorEnvelope{
		Error: apierror.ToRestModel(apiError, requestId),
		Meta: &rest_model.Meta{
			APIEnrolmentVersion: version.GetApiVersion(),
			APIVersion:          version.GetApiEnrollmentVersion(),
		},
	}

	if canRespondWithJson(r) {
		producer = runtime.JSONProducer()
		w.Header().Set("content-type", "application/json")
	}

	w.WriteHeader(apiError.Status)
	err := producer.Produce(w, data)

	if err != nil {
		pfxlog.Logger().WithError(err).WithField("requestId", requestId).Error("could not respond with error, producer errored")
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
