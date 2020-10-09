package events

import (
	"github.com/openziti/fabric/events"
	"github.com/openziti/foundation/util/cowslice"
)

func init() {
	events.RegisterEventType("edge.sessions", registerSessionEventHandler)
}

func AddSessionEventHandler(handler EdgeSessionEventHandler) {
	cowslice.Append(sessionEventHandlerRegistry, handler)
}

func RemoveSessionEventHandler(handler EdgeSessionEventHandler) {
	cowslice.Delete(sessionEventHandlerRegistry, handler)
}
