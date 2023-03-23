package actions

import (
	"fmt"
	"github.com/openziti/fablab/kernel/lib"
	"time"

	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/zitilab/models"
)

func NewStartAction() model.ActionBinder {
	action := &startAction{}
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
	workflow.AddAction(model.ActionFunc(func(m *model.Model) error {
		return m.ForEachComponent(".sdk-app", 5, func(c *model.Component) error {
			factory := lib.NewSshConfigFactory(c.GetHost())

			serviceCmd := fmt.Sprintf("nohup sudo /home/%s/fablab/bin/%s run -i /home/%s/fablab/cfg/%s > logs/%s.log 2>&1 &",
				factory.User(), c.BinaryName, factory.User(), c.PublicIdentity+".json", c.PublicIdentity)

			_, err := lib.RemoteExec(factory, serviceCmd)
			return err
		})
	}))
	return workflow
}

type startAction struct {
	//Metricbeat MetricbeatConfig
	//Consul     ConsulConfig

}

//type MetricbeatConfig struct {
//	ConfigPath string
//	DataPath   string
//	LogPath    string
//}
//
//type ConsulConfig struct {
//	ConfigDir  string
//	ServerAddr string
//	DataPath   string
//	LogPath    string
//}
