package main

import (
	"crypto/rand"
	"net/url"

	"github.com/openziti/fablab/kernel/model"
	runlevel_5_operation "github.com/openziti/ziti/network-tests/simple-transfer/stages/5_operation"
)

type stageFactory struct{}

func newStageFactory() model.Factory {
	return &stageFactory{}
}

func (sf *stageFactory) Build(m *model.Model) error {
	//generate 10k random bytes
	data := make([]byte, 10000)
	rand.Read(data)

	m.AddOperatingStage(runlevel_5_operation.AssertEcho("#echo-client", url.QueryEscape(string(data))))

	return nil
}
