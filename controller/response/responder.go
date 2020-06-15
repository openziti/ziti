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
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/schema"
	"github.com/openziti/edge/internal/version"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/foundation/validation"
	"net/http"
)

//todo: rename to Responder, remove old Responder and RequestResponder
type Responder interface {
	Respond(data interface{}, httpStatus int)
	RespondWithOk(data interface{}, meta *rest_model.Meta)
	RespondWithEmptyOk()
	RespondWithError(err error)
	RespondWithApiError(apiError *apierror.ApiError)
	SetProducer(producer runtime.Producer)
	GetProducer() runtime.Producer
	RespondWithCouldNotReadBody(err error)
	RespondWithCouldNotParseBody(err error)
	RespondWithValidationErrors(errors *schema.ValidationErrors)
	RespondWithNotFound()
	RespondWithNotFoundWithCause(cause error)
	RespondWithFieldError(fe *validation.FieldError)
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
	responder.RespondWithApiError(apierror.NewCouldNotValidate(errors))
}

func (responder *ResponderImpl) RespondWithNotFound() {
	responder.RespondWithApiError(apierror.NewNotFound())
}

func (responder *ResponderImpl) RespondWithNotFoundWithCause(cause error) {
	apiErr := apierror.NewNotFound()
	apiErr.Cause = cause
	responder.RespondWithApiError(apiErr)
}

func (responder *ResponderImpl) RespondWithFieldError(fe *validation.FieldError) {
	responder.RespondWithApiError(apierror.NewField(apierror.NewFieldError(fe.Reason, fe.FieldName, fe.FieldValue)))
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
	Respond(responder.rc.ResponseWriter, responder.rc.Id, responder.GetProducer(), data, httpStatus)
}

func (responder *ResponderImpl) RespondWithError(err error) {
	RespondWithError(responder.rc.ResponseWriter, responder.rc.Id, responder.GetProducer(), err)
}

func (responder *ResponderImpl) RespondWithApiError(apiError *apierror.ApiError) {
	RespondWithApiError(responder.rc.ResponseWriter, responder.rc.Id, responder.GetProducer(), apiError)
}

func Respond(w http.ResponseWriter, requestId uuid.UUID, producer runtime.Producer, data interface{}, httpStatus int) {
	w.WriteHeader(httpStatus)

	err := producer.Produce(w, data)

	if err != nil {
		pfxlog.Logger().WithError(err).WithField("requestId", requestId).Error("could not respond, producer errored")
	}
}

func RespondWithError(w http.ResponseWriter, requestId uuid.UUID, producer runtime.Producer, err error) {
	var apiError *apierror.ApiError
	var ok bool

	if apiError, ok = err.(*apierror.ApiError); !ok {
		apiError = apierror.NewUnhandled(err)
	}

	RespondWithApiError(w, requestId, producer, apiError)
}

func RespondWithApiError(w http.ResponseWriter, requestId uuid.UUID, producer runtime.Producer, apiError *apierror.ApiError) {
	data := &rest_model.APIErrorEnvelope{
		Error: apiError.ToRestModel(requestId.String()),
		Meta: &rest_model.Meta{
			APIEnrolmentVersion: version.GetApiVersion(),
			APIVersion:          version.GetApiEnrollmentVersion(),
		},
	}

	w.WriteHeader(apiError.Status)
	err := producer.Produce(w, data)

	if err != nil {
		pfxlog.Logger().WithError(err).WithField("requestId", requestId).Error("could not respond with error, producer errored")
	}
}
