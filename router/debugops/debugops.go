package debugops

import (
	"github.com/openziti/ziti/router/fabric"
	"github.com/openziti/ziti/router"
)

const (
	DumpApiSessions byte = 128
)

func RegisterEdgeRouterAgentOps(router *router.Router, sm fabric.StateManager, debugEnabled bool) {
	router.RegisterAgentOp(DumpApiSessions, sm.DumpApiSessions)
}
