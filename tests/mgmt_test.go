//go:build apitests

package tests

import (
	"github.com/golang/protobuf/proto"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"testing"
	"time"
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

	badId, err := identity.LoadClientIdentity(
		"./testdata/valid_client_cert/client.cert",
		"./testdata/valid_client_cert/client.key",
		"./testdata/ca/intermediate/certs/ca-chain.cert.pem")

	ctx.Req.NoError(err)
	mgmtAddress, err := transport.ParseAddress("tls:localhost:10001")
	ctx.Req.NoError(err)
	dialer := channel2.NewClassicDialer(badId, mgmtAddress, nil)
	ch, err := channel2.NewChannel("mgmt", dialer, nil)
	ctx.Req.NoError(err)

	request := &mgmt_pb.ListServicesRequest{
		Query: "true",
	}
	body, err := proto.Marshal(request)
	ctx.Req.NoError(err)
	requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListServicesRequestType), body)
	responseMsg, err := ch.SendAndWaitWithTimeout(requestMsg, 5*time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(responseMsg.ContentType, int32(mgmt_pb.ContentType_ListServicesResponseType))
	response := &mgmt_pb.ListServicesResponse{}
	err = proto.Unmarshal(responseMsg.Body, response)
	ctx.Req.NoError(err)
}
