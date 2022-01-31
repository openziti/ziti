package xlink_transport

import (
	"github.com/openziti/channel"
	"github.com/openziti/fabric/router/xlink"
)

// BindHandlerFactory can be implemented and provided to the factory to perform channel binding and other channel setup
// tasks at accept time.
//
type BindHandlerFactory interface {
	NewBindHandler(xlink xlink.Xlink, latency bool, listenerSide bool) channel.BindHandler
}
