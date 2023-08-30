//go:build apitests

package tests

import (
	"fmt"
	"github.com/openziti/identity"
	"net/http"
	"testing"
	"time"
)

func Test_FabricAuthNoCert(t *testing.T) {
	ctx := NewFabricTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.requireRestPort(5 * time.Second)

	client := ctx.NewRestClient(nil)
	resp, err := client.R().Get("https://localhost:1281/fabric/v1/services")
	ctx.Req.True(err != nil || resp.IsError())
}

func Test_FabricAuthWithCertFromDifferentChain(t *testing.T) {
	ctx := NewFabricTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.requireRestPort(5 * time.Second)

	badId, err := identity.LoadClientIdentity(
		"./testdata/invalid_client_cert/client.cert",
		"./testdata/invalid_client_cert/client.key",
		"./testdata/ca/intermediate/certs/ca-chain.cert.pem")
	ctx.Req.NoError(err)

	client := ctx.NewRestClient(badId)
	resp, err := client.R().Get("https://localhost:1281/fabric/v1/services")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
}

func Test_ListFabricServicesWithValidCert(t *testing.T) {
	ctx := NewFabricTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.requireRestPort(5 * time.Second)
	client := ctx.NewRestClientWithDefaults()
	resp, err := client.R().Get("https://localhost:1281/fabric/v1/services")
	fmt.Println(resp.String())
	ctx.Req.NoError(err)
	ctx.Req.True(resp.IsSuccess())
}
