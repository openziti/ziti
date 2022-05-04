//go:build apitests

package tests

import (
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/fabric/router/xlink_transport"
	"github.com/openziti/fabric/tests/testutil"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/transport/v2"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
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
	return self.link
}

type testBindHandlerFactory struct{}

func (t testBindHandlerFactory) BindChannel(channel.Binding) error {
	return nil
}

func (t testBindHandlerFactory) NewBindHandler(xlink.Xlink, bool, bool) channel.BindHandler {
	return t
}

func Test_LinkWithValidCertFromUnknownChain(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	mgmtClient := ctx.createTestFabricRestClient()
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
	metricsRegistery := metrics.NewRegistry("test", nil)
	factory := xlink_transport.NewFactory(xla, testBindHandlerFactory{}, tcfg, router.NewLinkRegistry(), metricsRegistery)
	dialer, err := factory.CreateDialer(badId, nil, tcfg)
	ctx.Req.NoError(err)
	dial := &ctrl_pb.Dial{
		LinkId:       "testLinkId",
		Address:      "tls:127.0.0.1:6004",
		RouterId:     "002",
		LinkProtocol: "tls",
	}
	_, err = dialer.Dial(dial)
	ctx.Req.Error(err)
	ctx.Req.ErrorIs(err, io.EOF)
}

func Test_UnrequestedLinkFromValidRouter(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	mgmtClient := ctx.createTestFabricRestClient()
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

	metricsRegistery := metrics.NewRegistry("test", nil)
	factory := xlink_transport.NewFactory(xla, testBindHandlerFactory{}, tcfg, router.NewLinkRegistry(), metricsRegistery)
	dialer, err := factory.CreateDialer(router2Id, nil, tcfg)
	ctx.Req.NoError(err)
	dial := &ctrl_pb.Dial{
		LinkId:       "testLinkId",
		Address:      "tls:127.0.0.1:6004",
		RouterId:     "002",
		LinkProtocol: "tls",
	}
	_, err = dialer.Dial(dial)
	if err != nil {
		ctx.Req.ErrorIs(err, io.EOF, "unexpected error: %v", err)
	} else {
		for i := int32(0); i < 100 && err == nil; i++ {
			payload := &xgress.Payload{
				Header: xgress.Header{
					CircuitId: "hello",
				},
				Sequence: i,
				Headers:  nil,
				Data:     []byte{1, 2, 3, 4},
			}
			err = xla.getLink().SendPayload(payload)
			ctx.Req.NoError(err)
		}
	}
}

func Test_DuplicateLinks(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	mgmtClient := ctx.createTestFabricRestClient()
	mgmtClient.EnrollRouter("001", "router-1", "testdata/router/001-client.cert.pem")
	mgmtClient.EnrollRouter("002", "router-2", "testdata/router/002-client.cert.pem")
	ctx.Teardown()

	ctrlListener := ctx.NewControlChannelListener()
	ctx.startRouter(1)

	acceptControl := func(id string) (channel.Channel, *testutil.MessageCollector) {
		return testutil.AcceptControl(id, ctrlListener, ctx.Req)
	}

	router1cc, msgc1 := acceptControl("router-1")

	ctx.startRouter(2)

	router2cc, _ := acceptControl("router-2")

	dial1 := &ctrl_pb.Dial{
		LinkId:       uuid.NewString(),
		RouterId:     "002",
		Address:      "tls:localhost:6005",
		LinkProtocol: "tls",
	}

	// send initial link request. This should result in a new link
	err := protobufs.MarshalTyped(dial1).WithTimeout(time.Second).SendAndWaitForWire(router1cc)
	ctx.Req.NoError(err)

	msg, err := msgc1.Next(time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_LinkConnectedType), msg.ContentType)

	dial2 := &ctrl_pb.Dial{
		LinkId:       uuid.NewString(),
		RouterId:     "002",
		Address:      "tls:localhost:6005",
		LinkProtocol: "tls",
	}

	// send another link request. This should result in the router letting us know about the current link
	err = protobufs.MarshalTyped(dial2).WithTimeout(time.Second).SendAndWaitForWire(router1cc)
	ctx.Req.NoError(err)

	msg, err = msgc1.Next(time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_RouterLinksType), msg.ContentType)
	rl := &ctrl_pb.RouterLinks{}
	ctx.Req.NoError(proto.Unmarshal(msg.Body, rl))
	ctx.Req.Equal(1, len(rl.Links))
	ctx.Req.Equal(dial1.LinkId, rl.Links[0].Id)
	ctx.Req.Equal("002", rl.Links[0].DestRouterId)

	// disconnect and reconnect the router
	_ = router1cc.Close()
	router1cc, msgc1 = acceptControl("router-1")

	// The router should let us know about existing links
	msg, err = msgc1.Next(time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_RouterLinksType), msg.ContentType)
	rl = &ctrl_pb.RouterLinks{}
	ctx.Req.NoError(proto.Unmarshal(msg.Body, rl))
	ctx.Req.Equal(1, len(rl.Links))
	ctx.Req.Equal(dial1.LinkId, rl.Links[0].Id)
	ctx.Req.Equal("002", rl.Links[0].DestRouterId)

	// Try one more time, should get another router link notification
	err = protobufs.MarshalTyped(dial2).WithTimeout(time.Second).SendAndWaitForWire(router1cc)
	ctx.Req.NoError(err)

	msg, err = msgc1.Next(time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_RouterLinksType), msg.ContentType)
	rl = &ctrl_pb.RouterLinks{}
	ctx.Req.NoError(proto.Unmarshal(msg.Body, rl))
	ctx.Req.Equal(1, len(rl.Links))
	ctx.Req.Equal(dial1.LinkId, rl.Links[0].Id)
	ctx.Req.Equal("002", rl.Links[0].DestRouterId)

	ctx.Teardown()
	_ = router1cc.Close()
	_ = router2cc.Close()
	_ = ctrlListener.Close()
}

func Test_DuplicateLinkWithLinkCloseListener(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	mgmtClient := ctx.createTestFabricRestClient()
	mgmtClient.EnrollRouter("001", "router-1", "testdata/router/001-client.cert.pem")
	mgmtClient.EnrollRouter("002", "router-2", "testdata/router/002-client.cert.pem")
	ctx.Teardown()

	ctrlListener := ctx.NewControlChannelListener()
	defer func() { _ = ctrlListener.Close() }()
	ctx.startRouter(1)

	acceptControl := func(id string) (channel.Channel, *testutil.MessageCollector) {
		return testutil.AcceptControl(id, ctrlListener, ctx.Req)
	}

	router1cc, msgc1 := acceptControl("router-1")

	router2 := ctx.startRouter(2)

	router2cc, _ := acceptControl("router-2")

	dial1 := &ctrl_pb.Dial{
		LinkId:       uuid.NewString(),
		RouterId:     "002",
		Address:      "tls:localhost:6005",
		LinkProtocol: "tls",
	}

	// send initial link request. This should result in a new link
	err := protobufs.MarshalTyped(dial1).WithTimeout(time.Second).SendAndWaitForWire(router1cc)
	ctx.Req.NoError(err)

	msg, err := msgc1.Next(time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_LinkConnectedType), msg.ContentType)

	dial2 := &ctrl_pb.Dial{
		LinkId:       uuid.NewString(),
		RouterId:     "002",
		Address:      "tls:localhost:6005",
		LinkProtocol: "tls",
	}

	// send another link request. This should result in the router letting us know about the current link
	err = protobufs.MarshalTyped(dial2).WithTimeout(time.Second).SendAndWaitForWire(router1cc)
	ctx.Req.NoError(err)

	msg, err = msgc1.Next(time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_RouterLinksType), msg.ContentType)
	rl := &ctrl_pb.RouterLinks{}
	ctx.Req.NoError(proto.Unmarshal(msg.Body, rl))
	ctx.Req.Equal(1, len(rl.Links))
	ctx.Req.Equal(dial1.LinkId, rl.Links[0].Id)
	ctx.Req.Equal("002", rl.Links[0].DestRouterId)

	// The router should also send us a fault response for the dialed link
	msg, err = msgc1.Next(5 * time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_FaultType), msg.ContentType)
	fault := &ctrl_pb.Fault{}
	ctx.Req.NoError(proto.Unmarshal(msg.Body, fault))
	ctx.Req.Equal(ctrl_pb.FaultSubject_LinkFault, fault.Subject)
	ctx.Req.Equal(dial2.LinkId, fault.Id)

	// disconnect and reconnect the router
	log := pfxlog.Logger()
	log.Info("---------- router 2: stopping ------------------------")
	log.Infof("expecting close of link: %v", dial1.LinkId)
	ctx.Req.NoError(router2.Shutdown())
	ctx.Req.NoError(ctx.waitForPortClose("localhost:6005", 2*time.Second))
	log.Info("---------- router 2: stopped  ------------------------")
	router2 = ctx.startRouter(2)

	router2cc, _ = acceptControl("router-2")

	// The router should tell us about the faulty link
	msg, err = msgc1.Next(5 * time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_FaultType), msg.ContentType)
	fault = &ctrl_pb.Fault{}
	ctx.Req.NoError(proto.Unmarshal(msg.Body, fault))
	ctx.Req.Equal(ctrl_pb.FaultSubject_LinkFault, fault.Subject)
	ctx.Req.Equal(dial1.LinkId, fault.Id)

	// Try one more time, this time it should result in a new connection
	err = protobufs.MarshalTyped(dial2).WithTimeout(time.Second).SendAndWaitForWire(router1cc)
	ctx.Req.NoError(err)

	msg, err = msgc1.Next(time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_LinkConnectedType), msg.ContentType)
	linkConnected := &ctrl_pb.LinkConnected{}
	ctx.Req.NoError(proto.Unmarshal(msg.Body, linkConnected))
	ctx.Req.Equal(dial2.LinkId, linkConnected.Id)

	ctx.Teardown()
	_ = router1cc.Close()
	_ = router2cc.Close()
	_ = ctrlListener.Close()
}

func Test_DuplicateLinkWithLinkCloseDialer(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	mgmtClient := ctx.createTestFabricRestClient()
	mgmtClient.EnrollRouter("001", "router-1", "testdata/router/001-client.cert.pem")
	mgmtClient.EnrollRouter("002", "router-2", "testdata/router/002-client.cert.pem")
	ctx.Teardown()

	ctrlListener := ctx.NewControlChannelListener()
	router1 := ctx.startRouter(1)

	acceptControl := func(id string) (channel.Channel, *testutil.MessageCollector) {
		return testutil.AcceptControl(id, ctrlListener, ctx.Req)
	}

	router1cc, msgc1 := acceptControl("router-1")

	ctx.startRouter(2)

	router2cc, msgc2 := acceptControl("router-2")

	dial1 := &ctrl_pb.Dial{
		LinkId:       uuid.NewString(),
		RouterId:     "002",
		Address:      "tls:localhost:6005",
		LinkProtocol: "tls",
	}

	// send initial link request. This should result in a new link
	err := protobufs.MarshalTyped(dial1).WithTimeout(time.Second).SendAndWaitForWire(router1cc)
	ctx.Req.NoError(err)

	msg, err := msgc1.Next(time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_LinkConnectedType), msg.ContentType)

	dial2 := &ctrl_pb.Dial{
		LinkId:       uuid.NewString(),
		RouterId:     "002",
		Address:      "tls:localhost:6005",
		LinkProtocol: "tls",
	}

	// send another link request. This should result in the router letting us know about the current link
	err = protobufs.MarshalTyped(dial2).WithTimeout(time.Second).SendAndWaitForWire(router1cc)
	ctx.Req.NoError(err)

	msg, err = msgc1.Next(time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_RouterLinksType), msg.ContentType)
	rl := &ctrl_pb.RouterLinks{}
	ctx.Req.NoError(proto.Unmarshal(msg.Body, rl))
	ctx.Req.Equal(1, len(rl.Links))
	ctx.Req.Equal(dial1.LinkId, rl.Links[0].Id)
	ctx.Req.Equal("002", rl.Links[0].DestRouterId)

	// disconnect and reconnect the router
	ctx.Req.NoError(router1.Shutdown())
	ctx.Req.NoError(ctx.waitForPortClose("localhost:6004", 2*time.Second))
	router1 = ctx.startRouter(1)

	router1cc, msgc1 = acceptControl("router-1")

	// The router should tell us about the faulty link
	msg, err = msgc2.Next(5 * time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_FaultType), msg.ContentType)
	fault := &ctrl_pb.Fault{}
	ctx.Req.NoError(proto.Unmarshal(msg.Body, fault))
	ctx.Req.Equal(ctrl_pb.FaultSubject_LinkFault, fault.Subject)
	ctx.Req.Equal(dial1.LinkId, fault.Id)

	// Try one more time, this time it should result in a new connection
	err = protobufs.MarshalTyped(dial2).WithTimeout(time.Second).SendAndWaitForWire(router1cc)
	ctx.Req.NoError(err)

	msg, err = msgc1.Next(time.Second)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(ctrl_pb.ContentType_LinkConnectedType), msg.ContentType)
	linkConnected := &ctrl_pb.LinkConnected{}
	ctx.Req.NoError(proto.Unmarshal(msg.Body, linkConnected))
	ctx.Req.Equal(dial2.LinkId, linkConnected.Id)

	ctx.Teardown()
	_ = router1cc.Close()
	_ = router2cc.Close()
	_ = ctrlListener.Close()
}
