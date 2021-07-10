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

package routes

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_client_api_server/operations/posture_checks"
	"github.com/openziti/edge/rest_model"
	"time"
)

func init() {
	r := NewPostureResponseRouter()
	env.AddRouter(r)
}

type PostureResponseRouter struct {
	BasePath string
}

func NewPostureResponseRouter() *PostureResponseRouter {
	return &PostureResponseRouter{
		BasePath: "/" + EntityNamePostureResponse,
	}
}

func (r *PostureResponseRouter) Register(ae *env.AppEnv) {
	ae.ClientApi.PostureChecksCreatePostureResponseHandler = posture_checks.CreatePostureResponseHandlerFunc(func(params posture_checks.CreatePostureResponseParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ClientApi.PostureChecksCreatePostureResponseBulkHandler = posture_checks.CreatePostureResponseBulkHandlerFunc(func(params posture_checks.CreatePostureResponseBulkParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.CreateBulk(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})
}

func (r *PostureResponseRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params posture_checks.CreatePostureResponseParams) {
	Create(rc, rc, PostureResponseLinkFactory, func() (string, error) {
		apiPostureResponse := params.PostureResponse
		r.handlePostureResponse(ae, rc, apiPostureResponse)
		return "", nil
	})
}

func (r *PostureResponseRouter) CreateBulk(ae *env.AppEnv, rc *response.RequestContext, params posture_checks.CreatePostureResponseBulkParams) {
	for _, apiPostureResponse := range params.PostureResponse {
		r.handlePostureResponse(ae, rc, apiPostureResponse)
	}

	Create(rc, rc, PostureResponseLinkFactory, func() (string, error) {
		return "", nil
	})
}

func (r *PostureResponseRouter) handlePostureResponse(ae *env.AppEnv, rc *response.RequestContext, apiPostureResponse rest_model.PostureResponseCreate) {
	switch apiPostureResponse.(type) {
	case *rest_model.PostureResponseDomainCreate:
		apiPostureResponse := apiPostureResponse.(*rest_model.PostureResponseDomainCreate)

		postureResponse := &model.PostureResponse{
			PostureCheckId: *apiPostureResponse.ID(),
			TypeId:         string(apiPostureResponse.TypeID()),
			LastUpdatedAt:  time.Now(),
			TimedOut:       false,
		}

		subType := &model.PostureResponseDomain{
			Name: *apiPostureResponse.Domain,
		}

		subType.PostureResponse = postureResponse
		postureResponse.SubType = subType

		ae.Handlers.PostureResponse.Create(rc.Identity.Id, []*model.PostureResponse{postureResponse})

	case *rest_model.PostureResponseMacAddressCreate:
		apiPostureResponse := apiPostureResponse.(*rest_model.PostureResponseMacAddressCreate)
		postureResponse := &model.PostureResponse{
			PostureCheckId: *apiPostureResponse.ID(),
			TypeId:         string(apiPostureResponse.TypeID()),
			LastUpdatedAt:  time.Now(),
			TimedOut:       false,
		}

		subType := &model.PostureResponseMac{
			Addresses: apiPostureResponse.MacAddresses,
		}

		subType.PostureResponse = postureResponse
		postureResponse.SubType = subType

		ae.Handlers.PostureResponse.Create(rc.Identity.Id, []*model.PostureResponse{postureResponse})

	case *rest_model.PostureResponseProcessCreate:
		apiPostureResponse := apiPostureResponse.(*rest_model.PostureResponseProcessCreate)

		postureResponse := &model.PostureResponse{
			PostureCheckId: *apiPostureResponse.ID(),
			TypeId:         string(apiPostureResponse.TypeID()),
			LastUpdatedAt:  time.Now(),
			TimedOut:       false,
		}

		subType := &model.PostureResponseProcess{
			Path:               apiPostureResponse.Path,
			IsRunning:          apiPostureResponse.IsRunning,
			BinaryHash:         apiPostureResponse.Hash,
			SignerFingerprints: apiPostureResponse.SignerFingerprints,
		}

		subType.PostureResponse = postureResponse
		postureResponse.SubType = subType

		ae.Handlers.PostureResponse.Create(rc.Identity.Id, []*model.PostureResponse{postureResponse})
	case *rest_model.PostureResponseOperatingSystemCreate:
		apiPostureResponse := apiPostureResponse.(*rest_model.PostureResponseOperatingSystemCreate)

		postureResponse := &model.PostureResponse{
			PostureCheckId: *apiPostureResponse.ID(),
			TypeId:         string(apiPostureResponse.TypeID()),
			LastUpdatedAt:  time.Now(),
			TimedOut:       false,
		}

		subType := &model.PostureResponseOs{
			Type:    *apiPostureResponse.Type,
			Version: *apiPostureResponse.Version,
			Build:   apiPostureResponse.Build,
		}

		subType.PostureResponse = postureResponse
		postureResponse.SubType = subType

		ae.Handlers.PostureResponse.Create(rc.Identity.Id, []*model.PostureResponse{postureResponse})
	case *rest_model.PostureResponseEndpointStateCreate:
		apiPostureResponse := apiPostureResponse.(*rest_model.PostureResponseEndpointStateCreate)

		postureResponse := &model.PostureResponse{
			PostureCheckId: *apiPostureResponse.ID(),
			TypeId:         string(apiPostureResponse.TypeID()),
			LastUpdatedAt:  time.Now(),
			TimedOut:       false,
		}

		subType := &model.PostureResponseEndpointState{
			ApiSessionId: rc.ApiSession.Id,
		}
		now := time.Now().UTC()

		if apiPostureResponse.Woken {
			subType.WokenAt = &now
		}

		if apiPostureResponse.Unlocked {
			subType.UnlockedAt = &now
		}

		subType.PostureResponse = postureResponse
		postureResponse.SubType = subType

		ae.Handlers.PostureResponse.Create(rc.Identity.Id, []*model.PostureResponse{postureResponse})
	}
}
