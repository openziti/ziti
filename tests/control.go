package tests

import (
	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/config"
)

func (ctx *TestContext) NewControlChannelListener() channel.UnderlayListener {
	config, err := config.LoadConfig(ctx.configSet.CtrlConfig)
	ctx.Req.NoError(err)
	ctx.Req.NoError(config.Db.Close())

	versionHeader, err := versions.StdVersionEncDec.Encode(versions.NewDefaultVersionProvider().AsVersionInfo())
	ctx.Req.NoError(err)

	capabilityMask := capabilities.NewMask(
		capabilities.ControllerCreateTerminatorV2,
		capabilities.ControllerSingleRouterLinkSource,
		capabilities.ControllerSupportsJWTLegacySessions,
	)
	headers := map[int32][]byte{
		channel.HelloVersionHeader:                       versionHeader,
		int32(ctrl_pb.ControlHeaders_CapabilitiesHeader): capabilityMask.Bytes(),
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
