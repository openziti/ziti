package xlink_transport

import (
	"github.com/netfoundry/ziti-fabric/router/xlink"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type ChannelAccepter interface {
	AcceptChannel(xlink xlink.Xlink, ch channel2.Channel) error
}
