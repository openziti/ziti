//go:build apitests

package tests

import (
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"testing"
)

func Test_MgmtChannelInvalidClient(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	badId, err := identity.LoadClientIdentity(
		"./testdata/invalid_client_cert/client.cert",
		"./testdata/invalid_client_cert/client.key",
		"./testdata/ca/intermediate/certs/ca-chain.cert.pem")

	ctx.Req.NoError(err)
	mgmtAddress, err := transport.ParseAddress("tls:localhost:10001")
	ctx.Req.NoError(err)
	dialer := channel2.NewClassicDialer(badId, mgmtAddress, nil)
	_, err = channel2.NewChannel("mgmt", dialer, nil)
	ctx.Req.Error(err)
}

func Test_MgmtChannelValidClient(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	mc := ctx.createMgmtClient()
	mc.ListServices("true")
}
