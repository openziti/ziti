package routes

import (
	"errors"
	"fmt"
	"github.com/go-openapi/runtime/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	enrollment_client "github.com/openziti/edge-api/rest_client_api_server/operations/enrollment"
	enrollment_management "github.com/openziti/edge-api/rest_management_api_server/operations/enrollment"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/response"
	"net/http"
)

func init() {
	r := NewNetworkJwtRouter()
	env.AddRouter(r)
}

const (
	EntityNameNetworkJwt = "network-jwts"

	EnrollmentMethodNetwork = "network"

	DefaultNetworkJwtName = "default"
)

type NetworkJwtRoute struct {
	BasePath string
}

func NewNetworkJwtRouter() *NetworkJwtRoute {
	return &NetworkJwtRoute{
		BasePath: "/" + EntityNameNetworkJwt,
	}
}

func (r *NetworkJwtRoute) Register(ae *env.AppEnv) {

	ae.ManagementApi.EnrollmentListNetworkJWTsHandler = enrollment_management.ListNetworkJWTsHandlerFunc(func(params enrollment_management.ListNetworkJWTsParams) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ClientApi.EnrollmentListNetworkJWTsHandler = enrollment_client.ListNetworkJWTsHandlerFunc(func(params enrollment_client.ListNetworkJWTsParams) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.Always())
	})

}

var networkJwt string

func (r *NetworkJwtRoute) List(ae *env.AppEnv, rc *response.RequestContext) {

	if networkJwt == "" {
		issuer := fmt.Sprintf(`https://%s/`, ae.GetConfig().Edge.Api.Address)

		claims := &ziti.EnrollmentClaims{
			EnrollmentMethod: EnrollmentMethodNetwork,
			RegisteredClaims: jwt.RegisteredClaims{
				Audience: jwt.ClaimStrings{env.JwtAudEnrollment},
				Issuer:   issuer,
				Subject:  issuer,
				ID:       uuid.NewString(),
			},
		}

		signer, err := ae.GetEnrollmentJwtSigner()

		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not get enrollment signer to generate a network JWT")
			rc.RespondWithError(errors.New("could not determine signer"))
			return
		}

		jwtStr, genErr := signer.Generate(claims)

		if genErr != nil {
			networkJwt = ""
			pfxlog.Logger().WithError(genErr).Error("could not sign network JWT")
			rc.RespondWithError(errors.New("could not generate claims"))
			return
		}

		networkJwt = jwtStr
	}

	name := DefaultNetworkJwtName
	resp := rest_model.ListNetworkJWTsEnvelope{
		Data: rest_model.NetworkJWTList{
			&rest_model.NetworkJWT{
				Name:  &name,
				Token: &networkJwt,
			},
		},
		Meta: &rest_model.Meta{},
	}

	rc.Respond(resp, http.StatusOK)
}
