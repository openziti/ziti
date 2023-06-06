package zitilab

import (
	"fmt"
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
	"strings"
)

var _ model.ComponentType = (*EchoServerType)(nil)

type EchoServerMode int

type EchoServerType struct {
	Version   string
	LocalPath string
}

func (self *EchoServerType) Dump() any {
	return map[string]string{
		"type_id":    "echo-server",
		"version":    self.Version,
		"local_path": self.LocalPath,
	}
}

func (self *EchoServerType) StageFiles(r model.Run, c *model.Component) error {
	return stageziti.StageZitiOnce(r, c, self.Version, self.LocalPath)
}

func (self *EchoServerType) getProcessFilter(c *model.Component) func(string) bool {
	return getZitiProcessFilter(c, "echo-server")
}

func (self *EchoServerType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	factory := lib.NewSshConfigFactory(c.GetHost())
	pids, err := lib.FindProcesses(factory, self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *EchoServerType) Start(_ model.Run, c *model.Component) error {
	binaryName := "ziti"
	if self.Version != "" {
		binaryName += "-" + self.Version
	}

	factory := lib.NewSshConfigFactory(c.GetHost())

	binaryPath := fmt.Sprintf("/home/%s/fablab/bin/%s", factory.User(), binaryName)
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s.json", factory.User(), c.Id)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", factory.User(), c.Id)

	serviceCmd := fmt.Sprintf("nohup %s learn demo echo-server -i %s --cli-agent-alias %s > %s 2>&1 &",
		binaryPath, configPath, c.Id, logsPath)

	value, err := lib.RemoteExec(factory, serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *EchoServerType) Stop(_ model.Run, c *model.Component) error {
	factory := lib.NewSshConfigFactory(c.GetHost())
	return lib.RemoteKillFilterF(factory, self.getProcessFilter(c))
}
