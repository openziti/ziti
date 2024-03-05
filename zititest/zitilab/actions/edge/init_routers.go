package edge

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
)

func InitEdgeRouters(componentSpec string, concurrency int) model.Action {
	return &initEdgeRoutersAction{
		componentSpec: componentSpec,
		concurrency:   concurrency,
	}
}

func (action *initEdgeRoutersAction) Execute(run model.Run) error {
	if err := zitilib_actions.EdgeExec(run.GetModel(), "delete", "edge-router", "where", "true"); err != nil {
		pfxlog.Logger().WithError(err).Warn("unable to delete routers")
	}

	return component.ExecInParallel(action.componentSpec, action.concurrency, zitilab.RouterActionsCreateAndEnroll).Execute(run)
}

type initEdgeRoutersAction struct {
	componentSpec string
	concurrency   int
}
