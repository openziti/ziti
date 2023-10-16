package zitilib_actions

import (
	"fmt"
	"github.com/openziti/fablab/kernel/libssh"

	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
)

type metricbeatStart struct {
	hostSpec   string
	configPath string
	dataPath   string
	logPath    string
}

func StartMetricbeat(hostSpec, configPath, dataPath, logPath string) model.Action {
	return &metricbeatStart{
		hostSpec:   hostSpec,
		configPath: configPath,
		dataPath:   dataPath,
		logPath:    logPath,
	}
}

func (mbs *metricbeatStart) Execute(run model.Run) error {
	return run.GetModel().ForEachHost(mbs.hostSpec, 24, func(host *model.Host) error {
		ssh := host.NewSshConfigFactory()

		cmd := fmt.Sprintf("screen -d -m nohup metricbeat --path.config %s --path.data %s --path.logs %s 2>&1 &", mbs.configPath, mbs.dataPath, mbs.logPath)

		if output, err := libssh.RemoteExec(ssh, cmd); err != nil {
			logrus.Errorf("error starting metricbeat service [%s] (%v)", output, err)
			return err
		}
		return nil
	})
}
