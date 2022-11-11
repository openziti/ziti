package actions

import (
	"time"

	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/zitilab/models"
)

func NewStartAction(metricbeat MetricbeatConfig, consul ConsulConfig) model.ActionBinder {
	action := &startAction{
		Metricbeat: metricbeat,
		Consul:     consul,
	}
	return action.bind
}

func (a *startAction) bind(m *model.Model) model.Action {
	workflow := actions.Workflow()
	workflow.AddAction(component.Start("#ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(component.StartInParallel(models.EdgeRouterTag, 25))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	//workflow.AddAction(zitilib_actions.StartMetricbeat("*", a.Metricbeat.ConfigPath, a.Metricbeat.DataPath, a.Metricbeat.LogPath))
	//workflow.AddAction(zitilib_actions.StartConsul("*", a.Consul.ServerAddr, a.Consul.ConfigDir, a.Consul.DataPath, a.Consul.LogPath))
	//workflow.AddAction(semaphore.Sleep(2 * time.Second))
	//workflow.AddAction(util_actions.StartEchoServers("#echo-server"))
	//workflow.AddAction(semaphore.Sleep(2 * time.Second))

	return workflow
}

type startAction struct {
	Metricbeat MetricbeatConfig
	Consul     ConsulConfig
}

type MetricbeatConfig struct {
	ConfigPath string
	DataPath   string
	LogPath    string
}

type ConsulConfig struct {
	ConfigDir  string
	ServerAddr string
	DataPath   string
	LogPath    string
}
