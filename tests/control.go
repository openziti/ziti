package tests

import (
	"github.com/openziti/channel/v2"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/controller"
)

func (ctx *FabricTestContext) NewControlChannelListener() channel.UnderlayListener {
	config, err := controller.LoadConfig(FabricControllerConfFile)
	ctx.Req.NoError(err)
	ctx.Req.NoError(config.Db.Close())

	versionHeader, err := versions.StdVersionEncDec.Encode(versions.NewDefaultVersionProvider().AsVersionInfo())
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
