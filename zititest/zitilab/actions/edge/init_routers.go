package edge

import (
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
)

func InitEdgeRouters(componentSpec string, concurrency int) model.Action {
	return &initEdgeRoutersAction{
		componentSpec: componentSpec,
		concurrency:   concurrency,
	}
}

func (action *initEdgeRoutersAction) Execute(run model.Run) error {
	return component.ExecInParallel(action.componentSpec, action.concurrency, zitilab.RouterActionsCreateAndEnroll).Execute(run)
}

type initEdgeRoutersAction struct {
	componentSpec string
	concurrency   int
}
