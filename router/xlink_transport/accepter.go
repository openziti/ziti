package xlink_transport

import (
	"github.com/netfoundry/ziti-fabric/router/xlink"
	"github.com/netfoundry/ziti-foundation/channel2"
)

// ChannelAccepter can be implemented and provided to the factory to perform channel2 binding and other channel setup
// tasks at accept time.
//
type ChannelAccepter interface {
	AcceptChannel(xlink xlink.Xlink, ch channel2.Channel) error
}
