package main

import (
	"fmt"

	fablib_5_operation "github.com/openziti/fablab/kernel/lib/runlevel/5_operation"
	"github.com/openziti/fablab/kernel/model"
	zitilib_runlevel_5_operation "github.com/openziti/zitilab/runlevel/5_operation"
)

type stageFactory struct{}

func newStageFactory() model.Factory {
	return &stageFactory{}
}

func (sf *stageFactory) Build(m *model.Model) error {
	runPhase := fablib_5_operation.NewPhase()
	fmt.Println("Added echo stage!")

	m.AddOperatingStage(zitilib_runlevel_5_operation.EchoClient("#echo-client"))
	m.AddOperatingStage(runPhase)
	//m.AddOperatingStage(fablib_5_operation.Persist())
	return nil
}
