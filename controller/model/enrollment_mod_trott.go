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

package model

import (
	"fmt"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/rest_model"
	"strings"
	"time"
)

const (
	MethodEnrollTransitRouterOtt = "trott"
)

type EnrollModuleRouterOtt struct {
	env                  Env
	method               string
	fingerprintGenerator cert.FingerprintGenerator
}

func NewEnrollModuleTransitRouterOtt(env Env) *EnrollModuleRouterOtt {
	handler := &EnrollModuleRouterOtt{
		env:                  env,
		method:               MethodEnrollTransitRouterOtt,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}

	return handler
}

func (module *EnrollModuleRouterOtt) CanHandle(method string) bool {
	return method == module.method
}

func (module *EnrollModuleRouterOtt) Process(context EnrollmentContext) (*EnrollmentResult, error) {
	enrollment, err := module.env.GetHandlers().Enrollment.ReadByToken(context.GetToken())

	if err != nil {
		return nil, err
	}

	if enrollment == nil || enrollment.TransitRouterId == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	txRouter, _ := module.env.GetHandlers().TransitRouter.Read(*enrollment.TransitRouterId)

	if txRouter == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	if time.Now().After(*enrollment.ExpiresAt) {
		return nil, apierror.NewEnrollmentExpired()
	}
	enrollData := context.GetDataAsMap()

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

	cr.Subject.CommonName = txRouter.Id

	cltCert, err := module.env.GetControlClientCsrSigner().Sign([]byte(enrollData["certCsr"].(string)), so)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	cltPem, err := module.env.GetControlClientCsrSigner().ToPem(cltCert)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	cltFp := module.fingerprintGenerator.FromPem(cltPem)

	txRouter.IsVerified = true
	txRouter.Fingerprint = strings.ToLower(strings.Replace(cltFp, ":", "", -1))

	if err := module.env.GetHandlers().TransitRouter.Update(txRouter, true); err != nil {
		return nil, fmt.Errorf("could not update edge router: %s", err)
	}

	if err := module.env.GetHandlers().Enrollment.Delete(enrollment.Id); err != nil {
		return nil, fmt.Errorf("could not delete enrollment: %s", err)
	}

	content := &rest_model.EnrollmentCerts{
		Ca:         string(module.env.GetConfig().CaPems()),
		Cert:       string(cltPem),
		ServerCert: srvChain,
	}

	return &EnrollmentResult{
		Identity:      nil,
		Authenticator: nil,
		Content:       content,
		Status:        200,
	}, nil
}
