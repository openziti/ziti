package edge

import (
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
)

func ReEnrollIdentities(componentSpec string, concurrency int) model.Action {
	return &reEnrollIdentitiesAction{
		componentSpec: componentSpec,
		concurrency:   concurrency,
	}
}

func (action *reEnrollIdentitiesAction) Execute(run model.Run) error {
	return component.ExecInParallel(action.componentSpec, action.concurrency, zitilab.ZitiTunnelActionsReEnroll).Execute(run)
}

type reEnrollIdentitiesAction struct {
	componentSpec string
	concurrency   int
}
