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

package model

import (
	"encoding/json"
	"fmt"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/validation"
	"github.com/netfoundry/ziti-edge/internal/cert"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/xeipuuv/gojsonschema"
	"strings"
	"time"
)

const (
	EdgeRouterEnrollmentCommonNameInvalidCode    = "EDGE_ROUTER_ENROLL_COMMON_NAME_INVALID"
	EdgeRouterEnrollmentCommonNameInvalidMessage = "The edge router CSR enrollment must have a common name that matches the edge router's id"

	MethodEnrollErOtt = "erott"
)

type EnrollModuleEr struct {
	env                  Env
	method               string
	fingerprintGenerator cert.FingerprintGenerator
}

func NewEnrollModuleEr(env Env) *EnrollModuleEr {
	handler := &EnrollModuleEr{
		env:                  env,
		method:               MethodEnrollErOtt,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}

	return handler
}

func (module *EnrollModuleEr) CanHandle(method string) bool {
	return method == module.method
}

func (module *EnrollModuleEr) Process(context EnrollmentContext) (*EnrollmentResult, error) {
	query := fmt.Sprintf(`isVerified = false and enrollmentToken = "%v"`, context.GetToken())
	gw, err := module.env.GetHandlers().EdgeRouter.ReadOneByQuery(query)

	if err != nil {
		return nil, err
	}

	if gw == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	if time.Now().After(*gw.EnrollmentExpiresAt) {
		return nil, apierror.NewEnrollmentExpired()
	}

	enrollData := context.GetDataAsMap()
	result, err := module.env.GetSchemas().GetEnrollErPost().Validate(gojsonschema.NewGoLoader(enrollData))

	if err != nil {
		return nil, err
	}

	if !result.Valid() {
		var errs []*validation.SchemaValidationError
		for _, re := range result.Errors() {
			errs = append(errs, validation.NewValidationError(re))
		}
		apiError := apierror.NewCouldNotValidate()

		if len(errs) > 0 {
			apiError.Cause = errs[0]
			apiError.AppendCause = true
		}

		return nil, apiError
	}

	sr, err := cert.ParseCsr([]byte(enrollData["serverCertCsr"].(string)))

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	so := &cert.SigningOpts{
		DNSNames:       sr.DNSNames,
		EmailAddresses: sr.EmailAddresses,
		IPAddresses:    sr.IPAddresses,
		URIs:           sr.URIs,
	}

	srvCert, err := module.env.GetApiServerCsrSigner().Sign([]byte(enrollData["serverCertCsr"].(string)), so)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	srvPem, err := module.env.GetApiServerCsrSigner().ToPem(srvCert)

	srvChain := string(srvPem) + module.env.GetApiServerCsrSigner().SigningCertPEM()

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	cr, err := cert.ParseCsr([]byte(enrollData["certCsr"].(string)))

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	so = &cert.SigningOpts{
		DNSNames:       cr.DNSNames,
		EmailAddresses: cr.EmailAddresses,
		IPAddresses:    cr.IPAddresses,
		URIs:           cr.URIs,
	}

	if cr.Subject.CommonName != gw.Id {

		return nil, &apierror.ApiError{
			Code:        EdgeRouterEnrollmentCommonNameInvalidCode,
			Message:     EdgeRouterEnrollmentCommonNameInvalidMessage,
			Status:      400,
			Cause:       nil,
			AppendCause: false,
		}

	}

	cltCert, err := module.env.GetControlClientCsrSigner().Sign([]byte(enrollData["certCsr"].(string)), so)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	cltPem, err := module.env.GetControlClientCsrSigner().ToPem(cltCert)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	cltFp := module.fingerprintGenerator.FromPem(cltPem)

	gw.IsVerified = true
	gw.EnrollmentCreatedAt = nil
	gw.EnrollmentExpiresAt = nil
	gw.EnrollmentJwt = nil
	gw.EnrollmentToken = nil
	gw.Fingerprint = &cltFp
	if err := module.env.GetHandlers().EdgeRouter.Update(gw, false); err != nil {
		return nil, fmt.Errorf("could not update edge router: %s", err)
	}

	if err := module.createRouter(cr.Subject.CommonName, cltFp); err != nil {
		return nil, err
	}

	content, err := json.Marshal(&map[string]interface{}{
		"serverCert": srvChain,
		"cert":       string(cltPem),
		"ca":         string(module.env.GetConfig().CaPems()),
	})

	if err != nil {
		return nil, err
	}

	return &EnrollmentResult{
		Identity:      nil,
		Authenticator: nil,
		Content:       content,
		ContentType:   "application/json",
		Status:        200,
	}, nil
}

func (module *EnrollModuleEr) createRouter(commonName string, fingerprint string) error {
	fgp := strings.Replace(strings.ToLower(fingerprint), ":", "", -1)
	r := network.NewRouter(commonName, fgp)
	return module.env.GetHostController().GetNetwork().CreateRouter(r)
}
