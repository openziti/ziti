package zitilab

import (
	"fmt"
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

func (self *EchoServerType) InitType(*model.Component) {
	canonicalizeGoAppVersion(&self.Version)
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
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *EchoServerType) Start(run model.Run, c *model.Component) error {
	user := c.GetHost().GetSshUser()

	binaryPath := getZitiBinaryPath(c, self.Version)
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s.json", user, c.Id)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	ha := ""
	if len(run.GetModel().SelectComponents(".ctrl")) > 1 {
		ha = "--ha"
	}

	serviceCmd := fmt.Sprintf("nohup %s demo echo-server --cli-agent-alias %s %s -i %s > %s 2>&1 &",
		binaryPath, c.Id, ha, configPath, logsPath)

	value, err := c.GetHost().ExecLogged(serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *EchoServerType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter(c))
}
