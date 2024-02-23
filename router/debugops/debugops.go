package debugops

import (
	"github.com/openziti/ziti/router"
	"github.com/openziti/ziti/router/state"
)

const (
	DumpApiSessions byte = 128
)

func RegisterEdgeRouterAgentOps(router *router.Router, sm state.Manager, debugEnabled bool) {
	router.RegisterAgentOp(DumpApiSessions, sm.DumpApiSessions)
}
