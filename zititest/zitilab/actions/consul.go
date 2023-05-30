package zitilib_actions

import (
	"fmt"

	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
)

type consulStart struct {
	hostSpec     string
	consulServer string
	configDir    string
	dataPath     string
	logFile      string
}

func StartConsul(hostSpec, consulServer, configDir, dataPath, logFile string) model.Action {
	return &consulStart{
		hostSpec:     hostSpec,
		consulServer: consulServer,
		configDir:    configDir,
		dataPath:     dataPath,
		logFile:      logFile,
	}
}

func (cs *consulStart) Execute(m *model.Model) error {
	return m.ForEachHost(cs.hostSpec, 24, func(c *model.Host) error {
		ssh := lib.NewSshConfigFactory(c)

		cmd := fmt.Sprintf("screen -d -m nohup consul agent -join %s -config-dir %s -data-dir %s -log-file %s 2>&1 &", cs.consulServer, cs.configDir, cs.dataPath, cs.logFile)

		if output, err := lib.RemoteExec(ssh, cmd); err != nil {
			logrus.Errorf("error starting consul service [%s] (%v)", output, err)
			return err
		}
		return nil
	})
}
