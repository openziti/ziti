package xlink_transport

import (
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
)

// ChannelAccepter can be implemented and provided to the factory to perform channel2 binding and other channel setup
// tasks at accept time.
//
type ChannelAccepter interface {
	AcceptChannel(xlink xlink.Xlink, payloadCh channel2.Channel, latency bool, listenerSide bool) error
}
