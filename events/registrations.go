package events

import (
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/util/cowslice"
)

func AddSessionEventHandler(handler network.SessionEventHandler) {
	cowslice.Append(network.SessionEventHandlerRegistry, handler)
}

func RemoveSessionEventHandler(handler network.SessionEventHandler) {
	cowslice.Delete(network.SessionEventHandlerRegistry, handler)
}

func AddTraceEventHandler(handler trace.EventHandler) {
	cowslice.Append(trace.EventHandlerRegistry, handler)
}

func RemoveTraceEventHandler(handler trace.EventHandler) {
	cowslice.Delete(trace.EventHandlerRegistry, handler)
}
