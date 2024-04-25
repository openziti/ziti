package debugops

import (
	"github.com/openziti/ziti/router"
)

const (
	DumpApiSessions byte = 128
)

func RegisterEdgeRouterAgentOps(router *router.Router, debugEnabled bool) {
	if sm := router.GetStateManager(); sm != nil {
		router.RegisterAgentOp(DumpApiSessions, sm.DumpApiSessions)
	}
}
