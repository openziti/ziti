/*
	Copyright NetFoundry Inc.

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
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/pkg/errors"
	"time"
)

const (
	EdgeRouterEnrollmentCommonNameInvalidCode    = "EDGE_ROUTER_ENROLL_COMMON_NAME_INVALID"
	EdgeRouterEnrollmentCommonNameInvalidMessage = "The edge router CSR enrollment must have a common name that matches the edge router's id"
	MethodEnrollEdgeRouterOtt                    = "erott"
)

type EnrollModuleEr struct {
	env                  Env
	method               string
	fingerprintGenerator cert.FingerprintGenerator
}

func NewEnrollModuleEdgeRouterOtt(env Env) *EnrollModuleEr {
	return &EnrollModuleEr{
		env:                  env,
		method:               MethodEnrollEdgeRouterOtt,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}
}

func (module *EnrollModuleEr) CanHandle(method string) bool {
	return method == module.method
}

func (module *EnrollModuleEr) Process(context EnrollmentContext) (*EnrollmentResult, error) {
	enrollment, err := module.env.GetManagers().Enrollment.ReadByToken(context.GetToken())

	if err != nil {
		return nil, err
	}

	if enrollment == nil || enrollment.EdgeRouterId == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	edgeRouter, _ := module.env.GetManagers().EdgeRouter.Read(*enrollment.EdgeRouterId)

	if edgeRouter == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	context.GetChangeContext().
		SetChangeAuthorType(change.AuthorTypeRouter).
		SetChangeAuthorId(edgeRouter.Id).
		SetChangeAuthorName(edgeRouter.Name)

	if time.Now().After(*enrollment.ExpiresAt) {
		return nil, apierror.NewEnrollmentExpired()
	}

	enrollData := context.GetData()

	serverCertCsrPem := enrollData.ServerCsrPem

	if len(serverCertCsrPem) == 0 {
		return nil, apierror.NewInvalidEnrollmentMissingCsr(errors.New("invalid server CSR"))
	}

	clientCertCsrPem := enrollData.ClientCsrPem

	if len(clientCertCsrPem) == 0 {
		return nil, apierror.NewInvalidEnrollmentMissingCsr(errors.New("invalid client CSR"))
	}

	serverCertRaw, err := module.ProcessServerCsrPem(serverCertCsrPem)

	if err != nil {
		apiError := apierror.NewCouldNotProcessCsr()
		apiError.Cause = errors.New("invalid server CSR")
		return nil, apiError
	}

	clientCertRaw, err := module.ProcessClientCsrPem(clientCertCsrPem, edgeRouter.Id)

	if err != nil {
		apiError := apierror.NewCouldNotProcessCsr()
		apiError.Cause = errors.New("invalid client CSR")
		return nil, apiError
	}

	serverCertPem, err := module.env.GetManagers().Enrollment.GetCertChainPem(serverCertRaw)
	if err != nil {
		return nil, err
	}

	clientCertPem, err := cert.RawToPem(clientCertRaw)
	if err != nil {
		return nil, err
	}

	module.env.GetApiServerCsrSigner()

	clientCertPemStr := string(clientCertPem)

	clientCertFingerprint := module.fingerprintGenerator.FromRaw(clientCertRaw)

	clientChainPem, err := module.env.GetManagers().Enrollment.GetCertChainPem(clientCertRaw)
	if err != nil {
		return nil, err
	}

	edgeRouter.CertPem = &clientCertPemStr
	edgeRouter.IsVerified = true
	edgeRouter.Fingerprint = &clientCertFingerprint
	if err := module.env.GetManagers().EdgeRouter.Update(edgeRouter, true, nil, context.GetChangeContext()); err != nil {
		return nil, fmt.Errorf("could not update edge router: %s", err)
	}

	if err := module.env.GetManagers().Enrollment.Delete(enrollment.Id, context.GetChangeContext()); err != nil {
		return nil, fmt.Errorf("could not delete enrollment: %s", err)
	}

	content := &rest_model.EnrollmentCerts{
		Ca:         string(module.env.GetConfig().Edge.CaPems()),
		Cert:       clientChainPem,
		ServerCert: serverCertPem,
	}

	return &EnrollmentResult{
		Identity:      nil,
		Authenticator: nil,
		Content:       content,
		Status:        200,
	}, nil
}

func (module *EnrollModuleEr) ProcessServerCsrPem(serverCertCsrPem []byte) ([]byte, error) {
	if len(serverCertCsrPem) == 0 {
		return nil, errors.New("empty server cert csr")
	}

	serverCsr, err := cert.ParseCsrPem(serverCertCsrPem)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	so := &cert.SigningOpts{
		DNSNames:       serverCsr.DNSNames,
		EmailAddresses: serverCsr.EmailAddresses,
		IPAddresses:    serverCsr.IPAddresses,
		URIs:           serverCsr.URIs,
	}

	serverCert, err := module.env.GetApiServerCsrSigner().SignCsr(serverCsr, so)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	return serverCert, nil
}

func (module *EnrollModuleEr) ProcessClientCsrPem(clientCertCsrPem []byte, edgeRouterId string) ([]byte, error) {
	if len(clientCertCsrPem) == 0 {
		return nil, errors.New("empty client cert csr")
	}

	clientCsr, err := cert.ParseCsrPem(clientCertCsrPem)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	so := &cert.SigningOpts{
		DNSNames:       clientCsr.DNSNames,
		EmailAddresses: clientCsr.EmailAddresses,
		IPAddresses:    clientCsr.IPAddresses,
		URIs:           clientCsr.URIs,
	}

	if clientCsr.Subject.CommonName != edgeRouterId {
		return nil, &errorz.ApiError{
			Code:        EdgeRouterEnrollmentCommonNameInvalidCode,
			Message:     EdgeRouterEnrollmentCommonNameInvalidMessage,
			Status:      400,
			Cause:       nil,
			AppendCause: false,
		}
	}

	cltCert, err := module.env.GetControlClientCsrSigner().SignCsr(clientCsr, so)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	return cltCert, nil
}
