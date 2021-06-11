package debugops

import (
	"github.com/openziti/edge/router/fabric"
	"github.com/openziti/fabric/router"
)

const (
	DumpApiSessions byte = 128
)

func RegisterEdgeRouterDebugOps(router *router.Router, sm fabric.StateManager) {
	router.RegisterDebugOp(DumpApiSessions, sm.DumpApiSessions)
}
