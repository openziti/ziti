package tests

import (
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/transport/v2"
)

func (ctx *TestContext) NewControlChannelListener() channel.UnderlayListener {
	config, err := controller.LoadConfig(ControllerConfFile)
	ctx.Req.NoError(err)

	versionHeader, err := versions.StdVersionEncDec.Encode(VersionProviderTest{}.AsVersionInfo())
	ctx.Req.NoError(err)
	headers := map[int32][]byte{
		channel.HelloVersionHeader: versionHeader,
	}

	ctrlChannelListenerConfig := channel.ListenerConfig{
		ConnectOptions:  config.Ctrl.Options.ConnectOptions,
		Headers:         headers,
		TransportConfig: transport.Configuration{"protocol": "ziti-ctrl"},
	}
	ctrlListener := channel.NewClassicListener(config.Id, config.Ctrl.Listener, ctrlChannelListenerConfig)
	ctx.Req.NoError(ctrlListener.Listen())
	return ctrlListener
}
