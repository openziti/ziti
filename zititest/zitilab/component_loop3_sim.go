/*
	Copyright 2019 NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package zitilab

import (
	"fmt"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
	"strings"
)

var _ model.ComponentType = (*Loop3SimType)(nil)

type Loop3SimType struct {
	LocalPath  string
	ConfigPath string
}

func (self *Loop3SimType) Label() string {
	return "loop3"
}

func (self *Loop3SimType) GetVersion() string {
	return "local"
}

func (self *Loop3SimType) Dump() any {
	return map[string]string{
		"type_id":     "simple-sim",
		"local_path":  self.LocalPath,
		"config_path": self.ConfigPath,
	}
}

func (self *Loop3SimType) StageFiles(r model.Run, c *model.Component) error {
	return stageziti.StageLocalOnce(r, "simple-sim", c, self.LocalPath)
}

func (self *Loop3SimType) getProcessFilter() func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "simple-sim")
	}
}

func (self *Loop3SimType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter())
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *Loop3SimType) Start(_ model.Run, c *model.Component) error {
	user := c.GetHost().GetSshUser()

	binaryPath := getBinaryPath(c, "simple-sim", "")
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)
	configPath := fmt.Sprintf("/home/%s/%s", user, self.ConfigPath)

	serviceCmd := fmt.Sprintf("%s %s  --log-formatter pfxlog > %s 2>&1 &",
		binaryPath, configPath, logsPath)

	value, err := c.Host.ExecLogged(
		"rm -f "+logsPath,
		serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *Loop3SimType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter())
}
