package events

import (
	"github.com/openziti/foundation/util/cowslice"
)

func AddSessionEventHandler(handler SessionEventHandler) {
	cowslice.Append(sessionEventHandlerRegistry, handler)
}

func RemoveSessionEventHandler(handler SessionEventHandler) {
	cowslice.Delete(sessionEventHandlerRegistry, handler)
}
