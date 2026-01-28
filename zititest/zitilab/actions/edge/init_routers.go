package edge

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/models"
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

type clientsInitializer func(run model.Run) (*zitirest.Clients, error)

func InitEdgeRoutersWithClients(componentSpec string, concurrency int, clientsF clientsInitializer) model.Action {
	return &initEdgeRoutersWithClientsAction{
		componentSpec: componentSpec,
		concurrency:   concurrency,
		clientsF:      clientsF,
	}
}

func (action *initEdgeRoutersWithClientsAction) Execute(run model.Run) error {
	clients, err := action.clientsF(run)
	if err != nil {
		return err
	}
	if err := zitilib_actions.EdgeExec(run.GetModel(), "delete", "edge-router", "where", "true"); err != nil {
		pfxlog.Logger().WithError(err).Warn("unable to delete routers")
	}

	var tasks []parallel.LabeledTask
	for _, c := range run.GetModel().SelectComponents(action.componentSpec) {
		if v, ok := c.Type.(*zitilab.RouterType); ok {
			tasks = append(tasks, v.CreateAndEnrollTasks(run, c, clients)...)
		}
	}

	return parallel.ExecuteLabeled(tasks, int64(action.concurrency), models.RetryPolicy)
}

type initEdgeRoutersWithClientsAction struct {
	componentSpec string
	concurrency   int
	clientsF      clientsInitializer
}
