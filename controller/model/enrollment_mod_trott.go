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
	enrollment, err := module.env.GetManagers().Enrollment.ReadByToken(context.GetToken())

	if err != nil {
		return nil, err
	}

	if enrollment == nil || enrollment.TransitRouterId == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	txRouter, _ := module.env.GetManagers().TransitRouter.Read(*enrollment.TransitRouterId)

	if txRouter == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	if time.Now().After(*enrollment.ExpiresAt) {
		return nil, apierror.NewEnrollmentExpired()
	}
	enrollData := context.GetDataAsMap()

	serverCsr, err := cert.ParseCsrPem([]byte(enrollData["serverCertCsr"].(string)))

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	signingOpts := &cert.SigningOpts{
		DNSNames:       serverCsr.DNSNames,
		EmailAddresses: serverCsr.EmailAddresses,
		IPAddresses:    serverCsr.IPAddresses,
		URIs:           serverCsr.URIs,
	}

	srvCert, err := module.env.GetApiServerCsrSigner().SignCsr(serverCsr, signingOpts)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	srvPem, err := cert.RawToPem(srvCert)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	clientCsr, err := cert.ParseCsrPem([]byte(enrollData["certCsr"].(string)))

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	signingOpts = &cert.SigningOpts{
		DNSNames:       clientCsr.DNSNames,
		EmailAddresses: clientCsr.EmailAddresses,
		IPAddresses:    clientCsr.IPAddresses,
		URIs:           clientCsr.URIs,
	}

	clientCsr.Subject.CommonName = txRouter.Id

	cltCert, err := module.env.GetControlClientCsrSigner().SignCsr(clientCsr, signingOpts)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	cltPem, err := cert.RawToPem(cltCert)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	cltFp := module.fingerprintGenerator.FromPem(cltPem)

	txRouter.IsVerified = true
	txRouter.Fingerprint = &cltFp

	if err := module.env.GetManagers().TransitRouter.Update(txRouter, true); err != nil {
		return nil, fmt.Errorf("could not update edge router: %s", err)
	}

	if err := module.env.GetManagers().Enrollment.Delete(enrollment.Id); err != nil {
		return nil, fmt.Errorf("could not delete enrollment: %s", err)
	}

	content := &rest_model.EnrollmentCerts{
		Ca:         string(module.env.GetConfig().CaPems()),
		Cert:       string(cltPem),
		ServerCert: string(srvPem),
	}

	return &EnrollmentResult{
		Identity:      nil,
		Authenticator: nil,
		Content:       content,
		Status:        200,
	}, nil
}
