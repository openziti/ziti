package routes

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"time"
)

const EntityNameTransitRouter = "transit-routers"

func NewTransitRouterEntityRef(entity *model.TransitRouter) *EntityApiRef {
	links := &response.Links{
		"self": NewTransitRouterLink(entity.Id),
	}

	return &EntityApiRef{
		Entity: EntityNameTransitRouter,
		Id:     entity.Id,
		Name:   &entity.Name,
		Links:  links,
	}
}

func NewTransitRouterLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameTransitRouter, id))
}

type TransitRouterApi struct {
	Name string                 `json:"name"`
	Tags map[string]interface{} `json:"tags"`
}

func (i TransitRouterApi) ToModel(id string) *model.TransitRouter {
	ret := &model.TransitRouter{
		BaseEntity: models.BaseEntity{
			Tags: i.Tags,
			Id:   id,
		},
		Name: i.Name,
	}
	return ret
}

type TransitRouterApiList struct {
	*env.BaseApi
	Name                string     `json:"name"`
	Fingerprint         string     `json:"fingerprint"`
	IsVerified          bool       `json:"isVerified"`
	IsOnline            bool       `json:"isOnline"`
	EnrollmentToken     *string    `json:"enrollmentToken"`
	EnrollmentJwt       *string    `json:"enrollmentJwt"`
	EnrollmentCreatedAt *time.Time `json:"enrollmentCreatedAt"`
	EnrollmentExpiresAt *time.Time `json:"enrollmentExpiresAt"`
}

func (e TransitRouterApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (e TransitRouterApiList) PopulateLinks() {
	if e.Links == nil {
		e.Links = &response.Links{
			EntityNameSelf: e.GetSelfLink(),
		}
	}
}

func (e TransitRouterApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameTransitRouter,
		Name:   nil,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func (e TransitRouterApiList) BuildSelfLink(id string) *response.Link {
	return NewTransitRouterLink(id)
}

func MapTransitRouterToApiEntity(appEnv *env.AppEnv, _ *response.RequestContext, entity models.Entity) (BaseApiEntity, error) {
	txRouter, ok := entity.(*model.TransitRouter)

	if !ok {
		err := fmt.Errorf("entity is not a transit router \"%s\"", entity.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	ret := &TransitRouterApiList{
		BaseApi:     env.FromBaseModelEntity(entity),
		Name:        txRouter.Name,
		Fingerprint: txRouter.Fingerprint,
		IsVerified:  txRouter.IsVerified,
		IsOnline:    appEnv.GetHandlers().Router.IsConnected(entity.GetId()),
	}

	if !txRouter.IsBase && !txRouter.IsVerified {
		var enrollments []*model.Enrollment

		err := appEnv.GetHandlers().TransitRouter.CollectEnrollments(txRouter.Id, func(entity *model.Enrollment) error {
			enrollments = append(enrollments, entity)
			return nil
		})

		if err != nil {
			return nil, err
		}

		if len(enrollments) != 1 {
			return nil, fmt.Errorf("expected enrollment not found for unverified transit router %s", txRouter.Id)
		}
		enrollment := enrollments[0]

		ret.EnrollmentExpiresAt = enrollment.ExpiresAt
		ret.EnrollmentCreatedAt = enrollment.IssuedAt
		ret.EnrollmentJwt = &enrollment.Jwt
		ret.EnrollmentToken = &enrollment.Token
	}

	ret.PopulateLinks()

	return ret, nil
}
