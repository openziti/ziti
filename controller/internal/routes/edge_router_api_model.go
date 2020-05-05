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
	"fmt"
	"github.com/go-openapi/strfmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/stringz"
)

const (
	EntityNameEdgeRouter = "edge-routers"
)

var EdgeRouterLinkFactory = NewEdgeRouterLinkFactory()

type EdgeRouterLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewEdgeRouterLinkFactory() *EdgeRouterLinkFactoryImpl {
	return &EdgeRouterLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameEdgeRouter),
	}
}

func (factory *EdgeRouterLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameEdgeRouterPolicy] = factory.NewNestedLink(entity, EntityNameEdgeRouterPolicy)
	return links
}

func MapCreateEdgeRouterToModel(router *rest_model.EdgeRouterCreate) *model.EdgeRouter {
	ret := &model.EdgeRouter{
		BaseEntity: models.BaseEntity{
			Tags: router.Tags,
		},
		Name:           stringz.OrEmpty(router.Name),
		RoleAttributes: router.RoleAttributes,
	}

	return ret
}

func MapUpdateEdgeRouterToModel(id string, router *rest_model.EdgeRouterUpdate) *model.EdgeRouter {
	ret := &model.EdgeRouter{
		BaseEntity: models.BaseEntity{
			Tags: router.Tags,
			Id:   id,
		},
		Name:           stringz.OrEmpty(router.Name),
		RoleAttributes: router.RoleAttributes,
	}

	return ret
}

func MapPatchEdgeRouterToModel(id string, router *rest_model.EdgeRouterPatch) *model.EdgeRouter {
	ret := &model.EdgeRouter{
		BaseEntity: models.BaseEntity{
			Tags: router.Tags,
			Id:   id,
		},
		Name:           router.Name,
		RoleAttributes: router.RoleAttributes,
	}

	return ret
}

func MapEdgeRouterToRestEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	router, ok := e.(*model.EdgeRouter)

	if !ok {
		err := fmt.Errorf("entity is not a EdgeRouter \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapEdgeRouterToRestModel(ae, router)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapEdgeRouterToRestModel(ae *env.AppEnv, router *model.EdgeRouter) (*rest_model.EdgeRouterDetail, error) {
	hostname := ""
	protocols := map[string]string{}

	onlineEdgeRouter := ae.Broker.GetOnlineEdgeRouter(router.Id)

	isOnline := onlineEdgeRouter != nil

	if isOnline {
		hostname = *onlineEdgeRouter.Hostname
		protocols = onlineEdgeRouter.EdgeRouterProtocols
	}

	ret := &rest_model.EdgeRouterDetail{
		BaseEntity:          BaseEntityToRestModel(router, EdgeRouterLinkFactory),
		Name:                &router.Name,
		RoleAttributes:      router.RoleAttributes,
		EnrollmentToken:     nil,
		EnrollmentCreatedAt: nil,
		EnrollmentExpiresAt: nil,
		EnrollmentJwt:       nil,
		IsOnline:            &isOnline,
		IsVerified:          &router.IsVerified,
		Fingerprint:         stringz.OrEmpty(router.Fingerprint),
		Hostname:            &hostname,
		SupportedProtocols:  protocols,
	}

	if !router.IsVerified {
		var enrollments []*model.Enrollment

		err := ae.GetHandlers().EdgeRouter.CollectEnrollments(router.Id, func(entity *model.Enrollment) error {
			enrollments = append(enrollments, entity)
			return nil
		})

		if err != nil {
			return nil, err
		}

		if len(enrollments) != 1 {
			return nil, fmt.Errorf("expected enrollment not found for unverified edge router %s", router.Id)
		}
		enrollment := enrollments[0]

		createdAt := strfmt.DateTime(enrollment.CreatedAt)
		expiresAt := strfmt.DateTime(*enrollment.ExpiresAt)

		ret.EnrollmentExpiresAt = &expiresAt
		ret.EnrollmentCreatedAt = &createdAt
		ret.EnrollmentJwt = &enrollment.Jwt
		ret.EnrollmentToken = &enrollment.Token
	}

	return ret, nil
}

//
//func NewEdgeRouterEntityRef(entity *model.EdgeRouter) *EntityApiRef {
//	links := &response.Links{
//		"self": NewEdgeRouterLink(entity.Id),
//	}
//
//	return &EntityApiRef{
//		Entity: EntityNameEdgeRouter,
//		Id:     entity.Id,
//		Name:   &entity.Name,
//		Links:  links,
//	}
//}
//
//func NewEdgeRouterLink(edgeRouterId string) *response.Link {
//	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameEdgeRouter, edgeRouterId))
//}
//
//type EdgeRouterEntityApiRef struct {
//	*EntityApiRef
//	Url *string `json:"url"`
//}
//
//type EdgeRouterApi struct {
//	Tags           map[string]interface{} `json:"tags"`
//	Name           *string                `json:"name"`
//	RoleAttributes []string               `json:"roleAttributes"`
//}
//
//func (i *EdgeRouterApi) ToModel(id string) *model.EdgeRouter {
//	result := &model.EdgeRouter{}
//	result.Id = id
//	result.Name = stringz.OrEmpty(i.Name)
//	result.RoleAttributes = i.RoleAttributes
//	result.Tags = i.Tags
//	return result
//}
//
//type EdgeRouterApiList struct {
//	*env.BaseApi
//	Name                *string           `json:"name"`
//	Fingerprint         *string           `json:"fingerprint"`
//	RoleAttributes      []string          `json:"roleAttributes"`
//	IsVerified          *bool             `json:"isVerified"`
//	IsOnline            *bool             `json:"isOnline"`
//	EnrollmentToken     *string           `json:"enrollmentToken"`
//	EnrollmentJwt       *string           `json:"enrollmentJwt"`
//	EnrollmentCreatedAt *time.Time        `json:"enrollmentCreatedAt"`
//	EnrollmentExpiresAt *time.Time        `json:"enrollmentExpiresAt"`
//	Hostname            *string           `json:"hostname"`
//	SupportedProtocols  map[string]string `json:"supportedProtocols"`
//}
//
//func (EdgeRouterApiList) BuildSelfLink(id string) *response.Link {
//	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameEdgeRouter, id))
//}
//
//func (e *EdgeRouterApiList) GetSelfLink() *response.Link {
//	return e.BuildSelfLink(e.Id)
//}
//
//func (e *EdgeRouterApiList) PopulateLinks() {
//	if e.Links == nil {
//		self := e.GetSelfLink()
//		e.Links = &response.Links{
//			EntityNameSelf:             self,
//			EntityNameEdgeRouterPolicy: response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameEdgeRouter)),
//		}
//	}
//}
//
//func (e *EdgeRouterApiList) ToEntityApiRef() *EntityApiRef {
//	e.PopulateLinks()
//	return &EntityApiRef{
//		Entity: EntityNameEdgeRouter,
//		Name:   e.Name,
//		Id:     e.Id,
//		Links:  e.Links,
//	}
//}
//
//func MapEdgeRouterToApiEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
//	i, ok := e.(*model.EdgeRouter)
//
//	if !ok {
//		err := fmt.Errorf("entity is not an edge router \"%s\"", e.GetId())
//		log := pfxlog.Logger()
//		log.Error(err)
//		return nil, err
//	}
//
//	al, err := MapEdgeRouterToApiList(ae, i)
//
//	if err != nil {
//		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
//		log := pfxlog.Logger()
//		log.Error(err)
//		return nil, err
//	}
//	return al, nil
//}
//
//func MapEdgeRouterToApiList(ae *env.AppEnv, i *model.EdgeRouter) (*EdgeRouterApiList, error) {
//	hostname := ""
//	protocols := map[string]string{}
//
//	onlineEdgeRouter := ae.Broker.GetOnlineEdgeRouter(i.Id)
//
//	isOnline := onlineEdgeRouter != nil
//
//	if isOnline {
//		hostname = *onlineEdgeRouter.Hostname
//		protocols = onlineEdgeRouter.EdgeRouterProtocols
//	}
//
//	ret := &EdgeRouterApiList{
//		BaseApi:             env.FromBaseModelEntity(i),
//		Name:                &i.Name,
//		RoleAttributes:      i.RoleAttributes,
//		EnrollmentToken:     nil,
//		EnrollmentCreatedAt: nil,
//		EnrollmentExpiresAt: nil,
//		EnrollmentJwt:       nil,
//		IsOnline:            &isOnline,
//		IsVerified:          &i.IsVerified,
//		Fingerprint:         i.Fingerprint,
//		Hostname:            &hostname,
//		SupportedProtocols:  protocols,
//	}
//
//	if !i.IsVerified {
//		var enrollments []*model.Enrollment
//
//		err := ae.GetHandlers().EdgeRouter.CollectEnrollments(i.Id, func(entity *model.Enrollment) error {
//			enrollments = append(enrollments, entity)
//			return nil
//		})
//
//		if err != nil {
//			return nil, err
//		}
//
//		if len(enrollments) != 1 {
//			return nil, fmt.Errorf("expected enrollment not found for unverified edge router %s", i.Id)
//		}
//		enrollment := enrollments[0]
//
//		ret.EnrollmentExpiresAt = enrollment.ExpiresAt
//		ret.EnrollmentCreatedAt = &enrollment.CreatedAt
//		ret.EnrollmentJwt = &enrollment.Jwt
//		ret.EnrollmentToken = &enrollment.Token
//	}
//
//	ret.PopulateLinks()
//
//	return ret, nil
//}
