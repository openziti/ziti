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
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/cert"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/pkg/errors"
)

const (
	EdgeRouterEnrollmentCommonNameInvalidCode    = "EDGE_ROUTER_ENROLL_COMMON_NAME_INVALID"
	EdgeRouterEnrollmentCommonNameInvalidMessage = "The edge router CSR enrollment must have a common name that matches the edge router's id"
	RouterCsrSpiffeIdNotAllowedCode              = "ROUTER_CSR_SPIFFE_ID_NOT_ALLOWED"
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

	serverCertRaw, err := module.ProcessServerCsrPem(serverCertCsrPem, edgeRouter.Id)

	if err != nil {
		apiError := apierror.NewCouldNotProcessCsr()
		apiError.Cause = err
		apiError.AppendCause = true
		return nil, apiError
	}

	clientCertRaw, err := module.ProcessClientCsrPem(clientCertCsrPem, edgeRouter.Id)

	if err != nil {
		apiError := apierror.NewCouldNotProcessCsr()
		apiError.Cause = err
		apiError.AppendCause = true
		return nil, apiError
	}

	serverCertPem, err := module.env.GetManagers().Enrollment.GetCertChainPem(serverCertRaw)
	if err != nil {
		return nil, err
	}

	module.env.GetApiServerCsrSigner()

	clientCertFingerprint := module.fingerprintGenerator.FromRaw(clientCertRaw)

	clientChainPem, err := module.env.GetManagers().Enrollment.GetCertChainPem(clientCertRaw)
	if err != nil {
		return nil, err
	}

	// The enrollment is consumed before anything is stored on the router. Deleting it is the one step that
	// concurrent requests presenting the same token compete on, so only the request whose delete succeeds
	// goes on to store its fingerprint. A request that loses is answered as if the token were unknown.
	if err := module.env.GetManagers().Enrollment.Delete(enrollment.Id, context.GetChangeContext()); err != nil {
		if boltz.IsErrNotFoundErr(err) {
			return nil, apierror.NewInvalidEnrollmentToken()
		}
		return nil, fmt.Errorf("could not delete enrollment: %s", err)
	}

	edgeRouter.CertPem = &clientChainPem
	edgeRouter.IsVerified = true
	edgeRouter.Fingerprint = &clientCertFingerprint
	if err := module.env.GetManagers().EdgeRouter.Update(edgeRouter, true, nil, context.GetChangeContext()); err != nil {
		return nil, fmt.Errorf("could not update edge router after its enrollment was consumed, the router must be re-enrolled: %s", err)
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

// ProcessServerCsrPem signs a router's server cert CSR with the API server signer, copying the CSR's SANs.
// It returns an error if the CSR requests a SPIFFE ID.
func (module *EnrollModuleEr) ProcessServerCsrPem(serverCertCsrPem []byte, routerId string) ([]byte, error) {
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

	uris, err := routerCsrUris(serverCsr, routerId, "server")
	if err != nil {
		return nil, err
	}

	so := &cert.SigningOpts{
		DNSNames:       serverCsr.DNSNames,
		EmailAddresses: serverCsr.EmailAddresses,
		IPAddresses:    serverCsr.IPAddresses,
		URIs:           uris,
	}

	serverCert, err := module.env.GetApiServerCsrSigner().SignCsr(serverCsr, so)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	return serverCert, nil
}

// ProcessClientCsrPem signs a router's client cert CSR with the control client signer, copying the CSR's
// SANs. It returns an error if the CSR's common name isn't edgeRouterId or the CSR requests a SPIFFE ID.
func (module *EnrollModuleEr) ProcessClientCsrPem(clientCertCsrPem []byte, edgeRouterId string) ([]byte, error) {
	if len(clientCertCsrPem) == 0 {
		return nil, errors.New("empty client cert csr")
	}

	clientCsr, err := cert.ParseCsrPem(clientCertCsrPem)

	if err != nil {
		return nil, apierror.NewCouldNotProcessCsr()
	}

	uris, err := routerCsrUris(clientCsr, edgeRouterId, "client")
	if err != nil {
		return nil, err
	}

	so := &cert.SigningOpts{
		DNSNames:       clientCsr.DNSNames,
		EmailAddresses: clientCsr.EmailAddresses,
		IPAddresses:    clientCsr.IPAddresses,
		URIs:           uris,
	}

	if clientCsr.Subject.CommonName != edgeRouterId {
		return nil, &errorz.ApiError{
			AppCode:     EdgeRouterEnrollmentCommonNameInvalidCode,
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

// routerCsrUris returns the URI SANs to copy from a router CSR into its signed cert. If the CSR requests a
// SPIFFE ID it logs a warning and returns an ApiError instead, since only the controller may assign one.
// routerId and csrType ("client" or "server") identify the CSR in the log and the error.
func routerCsrUris(csr *x509.CertificateRequest, routerId string, csrType string) ([]*url.URL, error) {
	for _, uri := range csr.URIs {
		if strings.EqualFold(uri.Scheme, "spiffe") {
			pfxlog.Logger().
				WithField("routerId", routerId).
				WithField("csrType", csrType).
				WithField("uri", uri.String()).
				Warn("rejected router CSR that requests a SPIFFE ID")
			return nil, &errorz.ApiError{
				AppCode: RouterCsrSpiffeIdNotAllowedCode,
				Message: fmt.Sprintf("the router %s CSR requests SPIFFE ID %s, router CSRs may not request SPIFFE IDs", csrType, uri),
				Status:  http.StatusBadRequest,
			}
		}
	}
	return csr.URIs, nil
}
