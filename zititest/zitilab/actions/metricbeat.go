package zitilib_actions

import (
	"fmt"

	"github.com/openziti/fablab/kernel/lib"
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

func (mbs *metricbeatStart) Execute(m *model.Model) error {
	return m.ForEachHost(mbs.hostSpec, 24, func(c *model.Host) error {
		ssh := lib.NewSshConfigFactory(c)

		cmd := fmt.Sprintf("screen -d -m nohup metricbeat --path.config %s --path.data %s --path.logs %s 2>&1 &", mbs.configPath, mbs.dataPath, mbs.logPath)

		if output, err := lib.RemoteExec(ssh, cmd); err != nil {
			logrus.Errorf("error starting metricbeat service [%s] (%v)", output, err)
			return err
		}
		return nil
	})
}
