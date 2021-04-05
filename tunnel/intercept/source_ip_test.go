package intercept

import (
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
)

type testProvider struct {
}

func (self *testProvider) PrepForUse(serviceId string) {
}

func (self *testProvider) GetCurrentIdentity() (*edge.CurrentIdentity, error) {
	return &edge.CurrentIdentity{
		Name: "foo.bar",
		AppData: map[string]interface{}{
			"srcip":      "123.456.789.10:5555",
			"sourceIp":   "15.14.13.12",
			"sourcePort": 1999,
		},
	}, nil
}

func (self *testProvider) TunnelService(service tunnel.Service, conn net.Conn, halfClose bool, appInfo []byte) error {
	panic("implement me")
}

func (self *testProvider) HostService(hostCtx tunnel.HostingContext) (tunnel.HostControl, error) {
	panic("implement me")
}

func Test_SourceIp(t *testing.T) {
	svc := &entities.Service{}

	sourceAddr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 5432}
	destAddr := &net.TCPAddr{IP: net.ParseIP("5.6.7.8"), Port: 80}

	req := require.New(t)

	poller := &ServiceListener{provider: &testProvider{}}
	req.NoError(poller.configureSourceAddrProvider(svc))
	req.Equal("", svc.GetSourceAddr(sourceAddr, destAddr))

	svc = &entities.Service{
		InterceptV1Config: &entities.InterceptV1Config{},
	}

	req.NoError(poller.configureSourceAddrProvider(svc))
	req.Equal("", svc.GetSourceAddr(sourceAddr, destAddr))

	testMatch := func(templ string, expected string) {
		svc.InterceptV1Config.SourceIp = &templ
		req.NoError(poller.configureSourceAddrProvider(svc))
		req.Equal(expected, svc.GetSourceAddr(sourceAddr, destAddr))
	}

	testMatch("$src_ip:$src_port", "1.2.3.4:5432")
	testMatch("$src_ip:$dst_port", "1.2.3.4:80")
	testMatch("$dst_ip:$src_port", "5.6.7.8:5432")
	testMatch("$dst_ip:$dst_port", "5.6.7.8:80")
	testMatch("$tunneler_id.name", "foo.bar")
	testMatch("$tunneler_id.appData[srcip]", "123.456.789.10:5555")
	testMatch("$tunneler_id.appData[sourceIp]:$tunneler_id.appData[sourcePort]", "15.14.13.12:1999")
}
