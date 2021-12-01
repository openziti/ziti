//go:build apitests

package tests

import (
	idlib "github.com/openziti/foundation/identity/identity"
	"net/http"
	"testing"
)

func Test_TestAuthWithCertFromDifferentChain(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	badId, err := idlib.LoadClientIdentity(
		"./testdata/invalid_client_cert/client.cert",
		"./testdata/invalid_client_cert/client.key",
		"./testdata/ca/intermediate/certs/ca-chain.cert.pem")
	ctx.Req.NoError(err)

	client := ctx.NewRestClient(badId)
	resp, err := client.R().Get("https://localhost:1281/fabric/v1/services")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
}

func Test_ListServicesWithValidCert(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	id, err := idlib.LoadClientIdentity(
		"./testdata/valid_client_cert/client.cert",
		"./testdata/valid_client_cert/client.key",
		"./testdata/ca/intermediate/certs/ca-chain.cert.pem")
	ctx.Req.NoError(err)

	client := ctx.NewRestClient(id)
	resp, err := client.R().Get("https://localhost:1281/fabric/v1/services")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
}

func Test_ListServicesWithEdgeAuth(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	ctx.RequireAdminManagementApiLogin()
	req := ctx.AdminManagementSession.newAuthenticatedRequest()
	resp, err := req.Get("https://localhost:1281/fabric/v1/services")
	ctx.Req.NoError(err)
	ctx.Req.True(resp.IsSuccess())
}
