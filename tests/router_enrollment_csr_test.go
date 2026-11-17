//go:build apitests

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

package tests

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/openziti/edge-api/rest_management_api_client/edge_router"
	"github.com/openziti/edge-api/rest_management_api_client/router"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/model"
)

// Test_RouterEnrollment_CsrSpiffeIds locks in that a router can't request a SPIFFE ID for its certs.
// Controllers and routers read a cert's SPIFFE ID to decide what its holder is (cluster member,
// identity, API session), so only the controller may assign one. Router enrollment and renewal reject
// such a CSR outright without using up the enrollment token, and keep accepting other URI SANs.
func Test_RouterEnrollment_CsrSpiffeIds(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	trustDomain := ctx.ControllerConfig.SpiffeIdTrustDomain.Hostname()

	t.Run("edge router enrollment rejects a client CSR that requests a SPIFFE ID", func(t *testing.T) {
		ctx.NextTest(t)

		createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{Name: util.Ptr(eid.New())},
		}, nil)
		ctx.Req.NoError(err)
		routerId := createResp.Payload.Data.ID

		routerDetail, err := mgmtClient.GetEdgeRouter(routerId)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(routerDetail.EnrollmentToken)
		token := *routerDetail.EnrollmentToken

		key := generateEcKey()
		controllerUri := &url.URL{Scheme: "spiffe", Host: trustDomain, Path: "/controller/" + eid.New()}
		clientCsr, err := routerCsrPem(key, routerId, []*url.URL{controllerUri}, nil)
		ctx.Req.NoError(err)
		serverCsr, err := routerCsrPem(key, routerId, nil, nil)
		ctx.Req.NoError(err)

		// Routers enroll through the legacy method endpoint, so that's the path under test.
		resp, err := ctx.newAnonymousClientApiRequest().
			SetBody(map[string]string{"certCsr": clientCsr, "serverCertCsr": serverCsr}).
			Post("/enroll?method=erott&token=" + token)
		ctx.Req.NoError(err)

		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
		errEnvelope := &rest_model.APIErrorEnvelope{}
		ctx.Req.NoError(json.Unmarshal(resp.Body(), errEnvelope))
		ctx.Req.NotNil(errEnvelope.Error.Cause, string(resp.Body()))
		ctx.Req.Equal(model.RouterCsrSpiffeIdNotAllowedCode, errEnvelope.Error.Cause.Code, string(resp.Body()))

		t.Run("the token still enrolls the router with CSRs that request only non-SPIFFE URIs", func(t *testing.T) {
			ctx.NextTest(t)

			keptUri := &url.URL{Scheme: "https", Host: "router.example.test", Path: "/" + routerId}
			clientCsr, err := routerCsrPem(key, routerId, []*url.URL{keptUri}, nil)
			ctx.Req.NoError(err)
			serverCsr, err := routerCsrPem(key, routerId, []*url.URL{keptUri}, []net.IP{net.ParseIP("127.0.0.1")})
			ctx.Req.NoError(err)

			resp, err := ctx.newAnonymousClientApiRequest().
				SetBody(map[string]string{"certCsr": clientCsr, "serverCertCsr": serverCsr}).
				Post("/enroll?method=erott&token=" + token)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "a rejected CSR should leave the enrollment token usable: %s", string(resp.Body()))
			var envelope struct {
				Data rest_model.EnrollmentCerts `json:"data"`
			}
			ctx.Req.NoError(json.Unmarshal(resp.Body(), &envelope))
			clientCerts, err := parsePEMBundle([]byte(envelope.Data.Cert))
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(clientCerts)
			serverCerts, err := parsePEMBundle([]byte(envelope.Data.ServerCert))
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(serverCerts)

			ctx.Req.Len(clientCerts[0].URIs, 1, "the client cert should carry the requested non-SPIFFE URI")
			ctx.Req.Equal(keptUri.String(), clientCerts[0].URIs[0].String())
			ctx.Req.Len(serverCerts[0].URIs, 1, "the server cert should carry the requested non-SPIFFE URI")
			ctx.Req.Equal(keptUri.String(), serverCerts[0].URIs[0].String())
			ctx.Req.Len(serverCerts[0].IPAddresses, 1, "the server cert should carry the requested IP SAN")
		})
	})

	t.Run("edge router enrollment rejects a server CSR that requests a SPIFFE ID", func(t *testing.T) {
		ctx.NextTest(t)

		createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{Name: util.Ptr(eid.New())},
		}, nil)
		ctx.Req.NoError(err)
		routerId := createResp.Payload.Data.ID

		routerDetail, err := mgmtClient.GetEdgeRouter(routerId)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(routerDetail.EnrollmentToken)

		key := generateEcKey()
		apiSessionUri := &url.URL{Scheme: "spiffe", Host: trustDomain, Path: "/identity/" + eid.New() + "/apiSession/" + eid.New()}
		clientCsr, err := routerCsrPem(key, routerId, nil, nil)
		ctx.Req.NoError(err)
		serverCsr, err := routerCsrPem(key, routerId, []*url.URL{apiSessionUri}, nil)
		ctx.Req.NoError(err)

		resp, err := ctx.newAnonymousClientApiRequest().
			SetBody(map[string]string{"certCsr": clientCsr, "serverCertCsr": serverCsr}).
			Post("/enroll?method=erott&token=" + *routerDetail.EnrollmentToken)
		ctx.Req.NoError(err)

		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
		errEnvelope := &rest_model.APIErrorEnvelope{}
		ctx.Req.NoError(json.Unmarshal(resp.Body(), errEnvelope))
		ctx.Req.NotNil(errEnvelope.Error.Cause, string(resp.Body()))
		ctx.Req.Equal(model.RouterCsrSpiffeIdNotAllowedCode, errEnvelope.Error.Cause.Code, string(resp.Body()))
	})

	t.Run("transit router enrollment rejects a client CSR that requests a SPIFFE ID", func(t *testing.T) {
		ctx.NextTest(t)

		createResp, err := mgmtClient.API.Router.CreateTransitRouter(&router.CreateTransitRouterParams{
			Router: &rest_model.RouterCreate{Name: util.Ptr(eid.New())},
		}, nil)
		ctx.Req.NoError(err)
		routerId := createResp.Payload.Data.ID

		detailResp, err := mgmtClient.API.Router.DetailTransitRouter(&router.DetailTransitRouterParams{ID: routerId}, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(detailResp.Payload.Data.EnrollmentToken)
		token := *detailResp.Payload.Data.EnrollmentToken

		// Even the router's own would-be SPIFFE ID is refused: only the controller assigns one.
		key := generateEcKey()
		routerUri := &url.URL{Scheme: "spiffe", Host: trustDomain, Path: "/router/" + routerId}
		clientCsr, err := routerCsrPem(key, routerId, []*url.URL{routerUri}, nil)
		ctx.Req.NoError(err)
		serverCsr, err := routerCsrPem(key, routerId, nil, nil)
		ctx.Req.NoError(err)

		// Transit router enrollment has no typed client operation, so it goes through the legacy
		// method endpoint the router itself uses.
		resp, err := ctx.newAnonymousClientApiRequest().
			SetBody(map[string]string{"certCsr": clientCsr, "serverCertCsr": serverCsr}).
			Post("/enroll?method=trott&token=" + token)
		ctx.Req.NoError(err)

		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
		errEnvelope := &rest_model.APIErrorEnvelope{}
		ctx.Req.NoError(json.Unmarshal(resp.Body(), errEnvelope))
		ctx.Req.NotNil(errEnvelope.Error.Cause, string(resp.Body()))
		ctx.Req.Equal(model.RouterCsrSpiffeIdNotAllowedCode, errEnvelope.Error.Cause.Code, string(resp.Body()))

		t.Run("the token still enrolls the router with CSRs that request no SPIFFE ID", func(t *testing.T) {
			ctx.NextTest(t)

			clientCsr, err := routerCsrPem(key, routerId, nil, nil)
			ctx.Req.NoError(err)
			serverCsr, err := routerCsrPem(key, routerId, nil, nil)
			ctx.Req.NoError(err)

			resp, err := ctx.newAnonymousClientApiRequest().
				SetBody(map[string]string{"certCsr": clientCsr, "serverCertCsr": serverCsr}).
				Post("/enroll?method=trott&token=" + token)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "a rejected CSR should leave the enrollment token usable: %s", string(resp.Body()))
		})
	})

	t.Run("transit router enrollment rejects a server CSR that requests a SPIFFE ID", func(t *testing.T) {
		ctx.NextTest(t)

		createResp, err := mgmtClient.API.Router.CreateTransitRouter(&router.CreateTransitRouterParams{
			Router: &rest_model.RouterCreate{Name: util.Ptr(eid.New())},
		}, nil)
		ctx.Req.NoError(err)
		routerId := createResp.Payload.Data.ID

		detailResp, err := mgmtClient.API.Router.DetailTransitRouter(&router.DetailTransitRouterParams{ID: routerId}, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(detailResp.Payload.Data.EnrollmentToken)

		key := generateEcKey()
		controllerUri := &url.URL{Scheme: "spiffe", Host: trustDomain, Path: "/controller/" + eid.New()}
		clientCsr, err := routerCsrPem(key, routerId, nil, nil)
		ctx.Req.NoError(err)
		serverCsr, err := routerCsrPem(key, routerId, []*url.URL{controllerUri}, nil)
		ctx.Req.NoError(err)

		resp, err := ctx.newAnonymousClientApiRequest().
			SetBody(map[string]string{"certCsr": clientCsr, "serverCertCsr": serverCsr}).
			Post("/enroll?method=trott&token=" + *detailResp.Payload.Data.EnrollmentToken)
		ctx.Req.NoError(err)

		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
		errEnvelope := &rest_model.APIErrorEnvelope{}
		ctx.Req.NoError(json.Unmarshal(resp.Body(), errEnvelope))
		ctx.Req.NotNil(errEnvelope.Error.Cause, string(resp.Body()))
		ctx.Req.Equal(model.RouterCsrSpiffeIdNotAllowedCode, errEnvelope.Error.Cause.Code, string(resp.Body()))
	})

	t.Run("edge router cert renewal rejects a CSR that requests a SPIFFE ID", func(t *testing.T) {
		ctx.NextTest(t)

		createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{Name: util.Ptr(eid.New())},
		}, nil)
		ctx.Req.NoError(err)
		routerId := createResp.Payload.Data.ID

		routerDetail, err := mgmtClient.GetEdgeRouter(routerId)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(routerDetail.EnrollmentToken)

		key := generateEcKey()
		clientCsr, err := routerCsrPem(key, routerId, nil, nil)
		ctx.Req.NoError(err)
		serverCsr, err := routerCsrPem(key, routerId, nil, nil)
		ctx.Req.NoError(err)

		enrollResp, err := ctx.newAnonymousClientApiRequest().
			SetBody(map[string]string{"certCsr": clientCsr, "serverCertCsr": serverCsr}).
			Post("/enroll?method=erott&token=" + *routerDetail.EnrollmentToken)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, enrollResp.StatusCode(), string(enrollResp.Body()))
		var envelope struct {
			Data rest_model.EnrollmentCerts `json:"data"`
		}
		ctx.Req.NoError(json.Unmarshal(enrollResp.Body(), &envelope))
		clientCerts, err := parsePEMBundle([]byte(envelope.Data.Cert))
		ctx.Req.NoError(err)

		controllerUri := &url.URL{Scheme: "spiffe", Host: trustDomain, Path: "/controller/" + eid.New()}
		renewKey := generateEcKey()
		renewClientCsr, err := routerCsrPem(renewKey, routerId, []*url.URL{controllerUri}, nil)
		ctx.Req.NoError(err)
		renewServerCsr, err := routerCsrPem(renewKey, routerId, nil, nil)
		ctx.Req.NoError(err)

		resp, err := ctx.newRequestWithClientCert(clientCerts, key).
			SetBody(map[string]string{"certCsr": renewClientCsr, "serverCertCsr": renewServerCsr}).
			Post("/enroll/extend/router")
		ctx.Req.NoError(err)

		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
		errEnvelope := &rest_model.APIErrorEnvelope{}
		ctx.Req.NoError(json.Unmarshal(resp.Body(), errEnvelope))
		ctx.Req.NotNil(errEnvelope.Error.Cause, string(resp.Body()))
		ctx.Req.Equal(model.RouterCsrSpiffeIdNotAllowedCode, errEnvelope.Error.Cause.Code, string(resp.Body()))
	})
}

// routerCsrPem returns a PEM encoded CSR signed by key, with the given common name, URI SANs and IP SANs.
func routerCsrPem(key crypto.PrivateKey, commonName string, uris []*url.URL, ips []net.IP) (string, error) {
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:     pkix.Name{CommonName: commonName},
		URIs:        uris,
		IPAddresses: ips,
	}, key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})), nil
}
