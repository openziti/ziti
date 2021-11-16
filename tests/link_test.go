package tests

import (
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/fabric/router/xlink_transport"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/sirupsen/logrus"
	"io"
	"testing"
	"time"
)

type testXlinkAcceptor struct {
	link xlink.Xlink
}

func (self *testXlinkAcceptor) Accept(l xlink.Xlink) error {
	logrus.Infof("xlink accepted: %+v", l)
	self.link = l
	return nil
}

func (self *testXlinkAcceptor) getLink() xlink.Xlink {
	time.Sleep(time.Second)
	return self.link
}

type testChannelAcceptor struct{}

func (t testChannelAcceptor) AcceptChannel(xlink xlink.Xlink, payloadCh channel2.Channel, latency bool, listenerSide bool) error {
	return nil
}

func Test_LinkWithValidCertFromUnknownChain(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	mgmtClient := ctx.createMgmtClient()
	mgmtClient.EnrollRouter("001", "router-1", "testdata/router/001-client.cert.pem")
	ctx.startRouter(1)
	ctx.Req.NoError(ctx.waitForPort("127.0.0.1:6004", 2*time.Second))

	badId, err := identity.LoadClientIdentity(
		"./testdata/invalid_client_cert/client.cert",
		"./testdata/invalid_client_cert/client.key",
		"./testdata/ca/intermediate/certs/ca-chain.cert.pem")
	ctx.Req.NoError(err)

	xla := &testXlinkAcceptor{}
	tcfg := transport.Configuration{
		"split": false,
	}
	factory := xlink_transport.NewFactory(xla, testChannelAcceptor{}, tcfg)
	dialer, err := factory.CreateDialer(badId, nil, tcfg)
	ctx.Req.NoError(err)
	linkId := badId.ShallowCloneWithNewToken("testLinkId")
	err = dialer.Dial("tls:127.0.0.1:6004", linkId, "router1")
	ctx.Req.Error(err)
	ctx.Req.ErrorIs(err, io.EOF)
}

func Test_UnrequestedLinkFromValidRouter(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	mgmtClient := ctx.createMgmtClient()
	mgmtClient.EnrollRouter("001", "router-1", "testdata/router/001-client.cert.pem")
	mgmtClient.EnrollRouter("002", "router-2", "testdata/router/002-client.cert.pem")
	ctx.startRouter(1)
	ctx.Req.NoError(ctx.waitForPort("127.0.0.1:6004", 2*time.Second))

	router2Id, err := identity.LoadClientIdentity(
		"./testdata/router/002-client.cert.pem",
		"./testdata/router/002.key.pem",
		"./testdata/ca/intermediate/certs/ca-chain.cert.pem")
	ctx.Req.NoError(err)

	xla := &testXlinkAcceptor{}
	tcfg := transport.Configuration{
		"split": false,
	}
	factory := xlink_transport.NewFactory(xla, testChannelAcceptor{}, tcfg)
	dialer, err := factory.CreateDialer(router2Id, nil, tcfg)
	ctx.Req.NoError(err)
	linkId := router2Id.ShallowCloneWithNewToken("testLinkId")
	err = dialer.Dial("tls:127.0.0.1:6004", linkId, "router1")
	if err != nil {
		ctx.Req.ErrorIs(err, io.EOF, "unexpected error: %v", err)
	} else {
		payload := &xgress.Payload{
			Header: xgress.Header{
				CircuitId: "hello",
			},
			Sequence: 0,
			Headers:  nil,
			Data:     []byte{1, 2, 3, 4},
		}
		err = xla.getLink().SendPayload(payload)
		ctx.Req.Error(err)
		ctx.Req.EqualError(err, "channel closed", "unexpected error: %v", err)
	}
}
