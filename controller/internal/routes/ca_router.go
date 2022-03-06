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
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/go-openapi/runtime/middleware"
	"github.com/golang-jwt/jwt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_management_api_server/operations/certificate_authority"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/pkg/errors"
	"net/http"
)

func init() {
	r := NewCaRouter()
	env.AddRouter(r)
}

type CaRouter struct {
	BasePath string
}

func NewCaRouter() *CaRouter {
	return &CaRouter{
		BasePath: "/" + EntityNameCa,
	}
}

func (r *CaRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.CertificateAuthorityDeleteCaHandler = certificate_authority.DeleteCaHandlerFunc(func(params certificate_authority.DeleteCaParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.CertificateAuthorityDetailCaHandler = certificate_authority.DetailCaHandlerFunc(func(params certificate_authority.DetailCaParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.CertificateAuthorityListCasHandler = certificate_authority.ListCasHandlerFunc(func(params certificate_authority.ListCasParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.CertificateAuthorityUpdateCaHandler = certificate_authority.UpdateCaHandlerFunc(func(params certificate_authority.UpdateCaParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.CertificateAuthorityCreateCaHandler = certificate_authority.CreateCaHandlerFunc(func(params certificate_authority.CreateCaParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.CertificateAuthorityPatchCaHandler = certificate_authority.PatchCaHandlerFunc(func(params certificate_authority.PatchCaParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.CertificateAuthorityVerifyCaHandler = certificate_authority.VerifyCaHandlerFunc(func(params certificate_authority.VerifyCaParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.VerifyCert(ae, rc, params)
		}, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.CertificateAuthorityGetCaJWTHandler = certificate_authority.GetCaJWTHandlerFunc(func(params certificate_authority.GetCaJWTParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.generateJwt(ae, rc)
		}, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

}

func (r *CaRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Ca, MapCaToRestEntity)
}

func (r *CaRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Ca, MapCaToRestEntity)
}

func (r *CaRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params certificate_authority.CreateCaParams) {
	Create(rc, rc, CaLinkFactory, func() (string, error) {
		return ae.Handlers.Ca.Create(MapCreateCaToModel(params.Ca))
	})
}

func (r *CaRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.Ca)
}

func (r *CaRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params certificate_authority.UpdateCaParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.Ca.Update(MapUpdateCaToModel(params.ID, params.Ca))
	})
}

func (r *CaRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params certificate_authority.PatchCaParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {
		return ae.Handlers.Ca.Patch(MapPatchCaToModel(params.ID, params.Ca), fields.FilterMaps("tags"))
	})
}

func (r *CaRouter) VerifyCert(ae *env.AppEnv, rc *response.RequestContext, params certificate_authority.VerifyCaParams) {
	id, err := rc.GetEntityId()

	if err != nil {
		log := pfxlog.Logger()
		err := fmt.Errorf("could not find id property: %v", response.IdPropertyName)
		log.Error(err)
		rc.RespondWithNotFound()
		return
	}

	ca, err := ae.Handlers.Ca.Read(id)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RespondWithNotFound()
			return
		}

		log := pfxlog.Logger()
		log.WithField("id", id).WithField("cause", err).
			Errorf("could not load identity by id [%s]: %s", id, err)
		rc.RespondWithError(err)
		return
	}

	if ca == nil {
		rc.RespondWithNotFound()
		return
	}

	if ca.IsVerified {

		rc.RespondWithApiError(apierror.NewCaAlreadyVerified())
		return
	}

	body := params.Certificate

	if len(body) == 0 {
		rc.RespondWithCouldNotParseBody(err)
		return
	}

	der, _ := pem.Decode([]byte(body))

	if der == nil {
		apiErr := apierror.NewCouldNotParseBody(nil)
		apiErr.Cause = err
		apiErr.AppendCause = true
		rc.RespondWithApiError(apiErr)
		return
	}

	if der.Type != "CERTIFICATE" {
		apiErr := apierror.NewExpectedPemBlockCertificate()
		apiErr.Cause = fmt.Errorf("ecountered PEM block type %s", der.Type)
		rc.RespondWithApiError(apiErr)
		return
	}

	cert, err := x509.ParseCertificate(der.Bytes)

	if err != nil {
		apiErr := apierror.NewCouldNotParseDerBlock()
		apiErr.AppendCause = true
		apiErr.Cause = err
		rc.RespondWithApiError(apiErr)
		return
	}

	if cert.Subject.CommonName != ca.VerificationToken {
		rc.RespondWithApiError(apierror.NewInvalidCommonName())
		return
	}

	caDer, _ := pem.Decode([]byte(ca.CertPem))

	caCert, err := x509.ParseCertificate(caDer.Bytes)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	_, err = cert.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})

	if err != nil {
		apiErr := apierror.NewFailedCertificateValidation()
		apiErr.Cause = err
		rc.RespondWithApiError(apiErr)
		return
	}

	err = ae.Handlers.Ca.Verified(ca)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}

func (r *CaRouter) generateJwt(ae *env.AppEnv, rc *response.RequestContext) {
	id, getErr := rc.GetEntityId()

	if getErr != nil {
		log := pfxlog.Logger()
		err := fmt.Errorf("could not find id property: %v", response.IdPropertyName)
		log.Error(err)
		rc.RespondWithNotFound()
		return
	}

	ca, loadErr := ae.Handlers.Ca.Read(id)

	if loadErr != nil {
		if boltz.IsErrNotFoundErr(loadErr) {
			rc.RespondWithNotFound()
			return
		}

		log := pfxlog.Logger()
		log.Errorf("could not load identity by id \"%s\": %s", id, loadErr)
		rc.RespondWithError(loadErr)
		return
	}

	if ca == nil {
		rc.RespondWithNotFound()
		return
	}

	var notAfter int64

	method := "ca"

	claims := &config.EnrollmentClaims{
		EnrollmentMethod: method,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: notAfter,
			Issuer:    fmt.Sprintf(`https://%s/`, ae.Config.Api.Address),
		},
	}
	mapClaims, err := claims.ToMapClaims()

	if err != nil {
		rc.RespondWithError(fmt.Errorf("could not convert CA enrollment claims to interface map: %s", err))
		return
	}

	jwt, genErr := ae.GetJwtSigner().Generate(ca.Id, ca.Id, mapClaims)

	if genErr != nil {
		rc.RespondWithError(errors.New("could not generate claims"))
		return
	}

	rc.ResponseWriter.Header().Set("content-type", "application/jwt")
	rc.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = rc.ResponseWriter.Write([]byte(jwt))
}
