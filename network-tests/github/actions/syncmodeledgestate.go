package actions

import (
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/zitilab/actions/edge"
	"github.com/openziti/zitilab/models"
)

func NewSyncModelEdgeStateAction() model.ActionBinder {
	action := &syncModelEdgeStateAction{}
	return action.bind
}

func (a *syncModelEdgeStateAction) bind(*model.Model) model.Action {
	workflow := actions.Workflow()
	workflow.AddAction(edge.Login("#ctrl"))
	workflow.AddAction(edge.SyncModelEdgeState(models.EdgeRouterTag))
	return workflow
}

type syncModelEdgeStateAction struct{}
