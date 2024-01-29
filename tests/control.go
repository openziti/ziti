package tests

import (
	"github.com/openziti/channel/v2"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/common/capabilities"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller"
	"math/big"
)

func (ctx *FabricTestContext) NewControlChannelListener() channel.UnderlayListener {
	config, err := controller.LoadConfig(FabricControllerConfFile)
	ctx.Req.NoError(err)
	ctx.Req.NoError(config.Db.Close())

	versionHeader, err := versions.StdVersionEncDec.Encode(versions.NewDefaultVersionProvider().AsVersionInfo())
	ctx.Req.NoError(err)

	capabilityMask := &big.Int{}
	capabilityMask.SetBit(capabilityMask, capabilities.ControllerCreateTerminatorV2, 1)
	capabilityMask.SetBit(capabilityMask, capabilities.ControllerSingleRouterLinkSource, 1)
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
