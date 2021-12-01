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

const EntityNameTransitRouter = "transit-routers"

var TransitRouterLinkFactory = NewBasicLinkFactory(EntityNameTransitRouter)

func MapCreateRouterToModel(router *rest_model.RouterCreate) *model.TransitRouter {
	ret := &model.TransitRouter{
		BaseEntity: models.BaseEntity{},
		Name:       stringz.OrEmpty(router.Name),
	}

	return ret
}

func MapUpdateTransitRouterToModel(id string, router *rest_model.RouterUpdate) *model.TransitRouter {
	ret := &model.TransitRouter{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(router.Tags),
			Id:   id,
		},
		Name: stringz.OrEmpty(router.Name),
	}

	return ret
}

func MapPatchTransitRouterToModel(id string, router *rest_model.RouterPatch) *model.TransitRouter {
	ret := &model.TransitRouter{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(router.Tags),
			Id:   id,
		},
		Name: router.Name,
	}

	return ret
}

func MapTransitRouterToRestEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	router, ok := e.(*model.TransitRouter)

	if !ok {
		err := fmt.Errorf("entity is not a TransitRouter \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapTransitRouterToRestModel(ae, router)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapTransitRouterToRestModel(ae *env.AppEnv, router *model.TransitRouter) (*rest_model.RouterDetail, error) {
	isConnected := ae.GetHandlers().Router.IsConnected(router.GetId())
	ret := &rest_model.RouterDetail{
		BaseEntity:            BaseEntityToRestModel(router, TransitRouterLinkFactory),
		Fingerprint:           router.Fingerprint,
		IsOnline:              &isConnected,
		IsVerified:            &router.IsVerified,
		Name:                  &router.Name,
		UnverifiedFingerprint: router.UnverifiedFingerprint,
		UnverifiedCertPem:     router.UnverifiedCertPem,
	}

	if !router.IsBase && !router.IsVerified {
		var enrollments []*model.Enrollment

		err := ae.GetHandlers().TransitRouter.CollectEnrollments(router.Id, func(entity *model.Enrollment) error {
			enrollments = append(enrollments, entity)
			return nil
		})

		if err != nil {
			return nil, err
		}

		if len(enrollments) != 1 {
			return nil, fmt.Errorf("expected enrollment not found for unverified transit router %s", router.Id)
		}
		enrollment := enrollments[0]

		expiresAt := strfmt.DateTime(*enrollment.ExpiresAt)
		createdAt := strfmt.DateTime(*enrollment.IssuedAt)

		ret.EnrollmentExpiresAt = &expiresAt
		ret.EnrollmentCreatedAt = &createdAt
		ret.EnrollmentJwt = &enrollment.Jwt
		ret.EnrollmentToken = &enrollment.Token
	}

	return ret, nil
}
