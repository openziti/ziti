package api_impl

import (
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"net/http"
)

func RespondWithCreatedId(responder api.Responder, id string, link rest_model.Link) {
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

func RespondWithOk(responder api.Responder, data interface{}, meta *rest_model.Meta) {
	responder.Respond(&rest_model.Empty{
		Data: data,
		Meta: meta,
	}, http.StatusOK)
}

type fabricResponseMapper struct{}

func (self fabricResponseMapper) EmptyOkData() interface{} {
	return &rest_model.Empty{
		Data: map[string]interface{}{},
		Meta: &rest_model.Meta{},
	}
}

func (self fabricResponseMapper) MapApiError(requestId string, apiError *errorz.ApiError) interface{} {
	return &rest_model.APIErrorEnvelope{
		Error: ToRestModel(apiError, requestId),
		Meta: &rest_model.Meta{
			APIEnrollmentVersion: ApiVersion,
			APIVersion:           ApiVersion,
		},
	}
}
