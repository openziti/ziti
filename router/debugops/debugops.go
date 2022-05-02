package debugops

import (
	"github.com/openziti/edge/router/fabric"
	"github.com/openziti/fabric/router"
)

const (
	DumpApiSessions byte = 128
)

func RegisterEdgeRouterAgentOps(router *router.Router, sm fabric.StateManager, debugEnabled bool) {
	router.RegisterAgentOp(DumpApiSessions, sm.DumpApiSessions)
}
