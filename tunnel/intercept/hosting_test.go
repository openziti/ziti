package intercept

import (
	"testing"

	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/tunnel/entities"
	"github.com/stretchr/testify/require"
)

func Test_DnsListenIdentityType(t *testing.T) {
	serviceId := uuid.NewString()
	svc := &entities.Service{
		ServiceDetail: rest_model.ServiceDetail{
			BaseEntity: rest_model.BaseEntity{
				ID: &serviceId,
			},
		},
	}
	provider := &testProvider{}
	currentIdentity, err := provider.GetCurrentIdentity()
	req := require.New(t)
	req.NoError(err)

	dns := "dns"
	testMatch := func(listenOptions *entities.HostV1ListenOptions, expected string) {
		hostTerminator := &entities.HostV1Config{ListenOptions: listenOptions}
		options, err := getDefaultOptions(svc, currentIdentity, hostTerminator)
		req.NoError(err)
		req.Equal(expected, options.Identity)
	}

	testMatch(&entities.HostV1ListenOptions{Identity: "MyHost.Example.COM"}, "MyHost.Example.COM")
	testMatch(&entities.HostV1ListenOptions{Identity: "MyHost.Example.COM", ListenIdentityType: &dns}, "myhost.example.com")

	currentIdentity.Name = util.Ptr("Foo.Bar")
	testMatch(&entities.HostV1ListenOptions{BindUsingEdgeIdentity: true}, "Foo.Bar")
	testMatch(&entities.HostV1ListenOptions{BindUsingEdgeIdentity: true, ListenIdentityType: &dns}, "foo.bar")
}
