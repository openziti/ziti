package main

import (
	"github.com/openziti/fablab/kernel/model"
)

type stageFactory struct{}

func newStageFactory() model.Factory {
	return &stageFactory{}
}

func (sf *stageFactory) Build(m *model.Model) error {
	//runPhase := fablib_5_operation.NewPhase()

	//generate 10k random bytes
	//data := make([]byte, 10000)
	//rand.Read(data)
	//
	//m.AddOperatingStage(fablib_5_operation.Iperf("test", "localhost:5432", ".iperf-server", ".iperf-client", 30))
	//m.AddOperatingStage(runPhase)
	//m.AddOperatingStage(fablib_5_operation.Persist())
	return nil
}
