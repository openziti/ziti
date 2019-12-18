/*
	Copyright 2019 Netfoundry, Inc.

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
	jwt2 "github.com/dgrijalva/jwt-go"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/netfoundry/ziti-sdk-golang/ziti/config"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
)

func init() {
	r := NewCaRouter()
	env.AddRouter(r)
}

type CaRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewCaRouter() *CaRouter {
	return &CaRouter{
		BasePath: "/" + EntityNameCa,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *CaRouter) Register(ae *env.AppEnv) {
	sr := registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())

	idUrlWithoutSlash := fmt.Sprintf("/{%s}/verify", response.IdPropertyName)
	idUrlWithSlash := fmt.Sprintf("/{%s}/verify/", response.IdPropertyName)
	verifyHandler := ae.WrapHandler(ir.VerifyCert, permissions.IsAdmin())
	sr.HandleFunc(idUrlWithoutSlash, verifyHandler).Methods(http.MethodPost)
	sr.HandleFunc(idUrlWithSlash, verifyHandler).Methods(http.MethodPost)

	getJwtWithSlash := fmt.Sprintf("/{%s}/jwt", response.IdPropertyName)
	getJwtWithoutSlash := fmt.Sprintf("/{%s}/jwt/", response.IdPropertyName)
	jwtHandler := ae.WrapHandler(ir.generateJwt, permissions.IsAdmin())
	sr.HandleFunc(getJwtWithSlash, jwtHandler).Methods(http.MethodGet)
	sr.HandleFunc(getJwtWithoutSlash, jwtHandler).Methods(http.MethodGet)
}

func (ir *CaRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Ca, MapCaToApiEntity)
}

func (ir *CaRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Ca, MapCaToApiEntity, ir.IdType)
}

func (ir *CaRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &CaApiCreate{}
	Create(rc, rc.RequestResponder, ae.Schemes.Ca.Post, apiEntity, (&CaApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.Ca.HandleCreate(apiEntity.ToModel())
	})
}

func (ir *CaRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.Ca)
}

func (ir *CaRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &CaApiUpdate{}
	Update(rc, ae.Schemes.Ca.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.Ca.HandleUpdate(apiEntity.ToModel(id))
	})
}

func (ir *CaRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &CaApiUpdate{}
	Patch(rc, ae.Schemes.Ca.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.Ca.HandlePatch(apiEntity.ToModel(id), fields.ConcatNestedNames())
	})
}

func (ir *CaRouter) VerifyCert(ae *env.AppEnv, rc *response.RequestContext) {
	id, err := rc.GetIdFromRequest(ir.IdType)

	if err != nil {
		log := pfxlog.Logger()
		err := fmt.Errorf("could not find id property: %v", response.IdPropertyName)
		log.Error(err)
		rc.RequestResponder.RespondWithNotFound()
		return
	}

	ca, err := ae.Handlers.Ca.HandleRead(id)

	if err != nil {
		if util.IsErrNotFoundErr(err) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		log := pfxlog.Logger()
		log.WithField("id", id).WithField("cause", err).
			Errorf("could not load identity by id [%s]: %s", id, err)
		rc.RequestResponder.RespondWithError(err)
		return
	}

	if ca == nil {
		rc.RequestResponder.RespondWithNotFound()
		return
	}

	if ca.IsVerified {

		rc.RequestResponder.RespondWithApiError(apierror.NewCaAlreadyVerified())
		return
	}

	body, err := ioutil.ReadAll(rc.Request.Body)

	if err != nil || len(body) == 0 {
		rc.RequestResponder.RespondWithCouldNotParseBody(err)
		return
	}

	der, _ := pem.Decode(body)

	if der == nil {
		apiErr := apierror.NewCouldNotParseBody()
		apiErr.Cause = err
		apiErr.AppendCause = true
		rc.RequestResponder.RespondWithApiError(apiErr)
		return
	}

	if der.Type != "CERTIFICATE" {
		apiErr := apierror.NewExpectedPemBlockCertificate()
		apiErr.Cause = fmt.Errorf("ecountered PEM block type %s", der.Type)
		rc.RequestResponder.RespondWithApiError(apiErr)
		return
	}

	cert, err := x509.ParseCertificate(der.Bytes)

	if err != nil {
		apiErr := apierror.NewCouldNotParseDerBlock()
		apiErr.AppendCause = true
		apiErr.Cause = err
		rc.RequestResponder.RespondWithApiError(apiErr)
		return
	}

	if cert.Subject.CommonName != ca.VerificationToken {
		rc.RequestResponder.RespondWithApiError(apierror.NewInvalidCommonName())
		return
	}

	caDer, _ := pem.Decode([]byte(ca.CertPem))

	caCert, err := x509.ParseCertificate(caDer.Bytes)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
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
		rc.RequestResponder.RespondWithApiError(apiErr)
		return
	}

	err = ae.Handlers.Ca.HandleVerified(ca)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	rc.RequestResponder.RespondWithOk(nil, nil)
}

func (ir *CaRouter) generateJwt(ae *env.AppEnv, rc *response.RequestContext) {
	id, getErr := rc.GetIdFromRequest(ir.IdType)

	if getErr != nil {
		log := pfxlog.Logger()
		err := fmt.Errorf("could not find id property: %v", response.IdPropertyName)
		log.Error(err)
		rc.RequestResponder.RespondWithNotFound()
		return
	}

	ca, loadErr := ae.Handlers.Ca.HandleRead(id)

	if loadErr != nil {
		if util.IsErrNotFoundErr(loadErr) {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		log := pfxlog.Logger()
		log.Errorf("could not load identity by id \"%s\": %s", id, loadErr)
		rc.RequestResponder.RespondWithError(loadErr)
		return
	}

	if ca == nil {
		rc.RequestResponder.RespondWithNotFound()
		return
	}

	var notAfter int64

	method := "ca"

	claims := &config.EnrollmentClaims{
		EnrollmentMethod: method,
		StandardClaims: jwt2.StandardClaims{
			ExpiresAt: notAfter,
			Issuer:    fmt.Sprintf(`https://%s/`, ae.Config.Api.Advertise),
		},
	}
	mapClaims, err := claims.ToMapClaims()

	if err != nil {
		rc.RequestResponder.RespondWithError(fmt.Errorf("could not convert CA enrollment claims to interface map: %s", err))
		return
	}

	jwt, genErr := ae.EnrollmentJwtGenerator.Generate(ca.Id, ca.Id, mapClaims)

	if genErr != nil {
		rc.RequestResponder.RespondWithError(errors.New("could not generate claims"))
		return
	}

	rc.ResponseWriter.Header().Set("Content-Type", "application/jwt")
	response.AddVersionHeader(rc.ResponseWriter)
	rc.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = rc.ResponseWriter.Write([]byte(jwt))
}
