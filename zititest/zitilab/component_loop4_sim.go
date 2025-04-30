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
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
	"io/fs"
	"strings"
)

var _ model.ComponentType = (*Loop4SimType)(nil)

type Loop4Mode int

func (self Loop4Mode) String() string {
	if self == Loop4Dialer {
		return "dialer"
	} else if self == Loop4Listener {
		return "listener"
	}
	panic(fmt.Errorf("unknown loop4 mode '%d'", self))
}

const (
	Loop4Dialer   Loop4Mode = 0
	Loop4Listener Loop4Mode = 1
)

type Loop4SimType struct {
	ConfigSourceFS fs.FS
	LocalPath      string
	ConfigName     string
	ConfigSource   string
	Mode           Loop4Mode
}

func (self *Loop4SimType) Label() string {
	return "loop4"
}

func (self *Loop4SimType) GetVersion() string {
	return "local"
}

func (self *Loop4SimType) Dump() any {
	return map[string]string{
		"type_id":       "ziti-traffic-test/loop4",
		"local_path":    self.LocalPath,
		"config_source": self.ConfigSource,
	}
}

func (self *Loop4SimType) StageFiles(r model.Run, c *model.Component) error {
	configSource := self.ConfigSource
	if configSource == "" {
		configSource = c.Id + ".yml.tmpl"
	}

	configName := self.GetConfigName(c)

	if err := lib.GenerateConfigForComponent(c, self.ConfigSourceFS, configSource, configName, r); err != nil {
		return err
	}

	return stageziti.StageLocalOnce(r, "ziti-traffic-test", c, self.LocalPath)
}

func (self *Loop4SimType) GetConfigName(c *model.Component) string {
	configName := self.ConfigName
	if configName == "" {
		configName = c.Id + ".yml"
	}
	return configName
}

func (self *Loop4SimType) getProcessFilter() func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "ziti-traffic-test")
	}
}

func (self *Loop4SimType) GetConfigPath(c *model.Component) string {
	return fmt.Sprintf("/home/%s/fablab/cfg/%s.yml", c.GetHost().GetSshUser(), c.Id)
}

func (self *Loop4SimType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter())
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *Loop4SimType) Start(_ model.Run, c *model.Component) error {
	user := c.GetHost().GetSshUser()

	binaryPath := getBinaryPath(c, "ziti-traffic-test", "")
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)
	configPath := self.GetConfigPath(c)

	serviceCmd := fmt.Sprintf("%s loop4 %s %s > %s 2>&1 &",
		binaryPath, self.Mode.String(), configPath, logsPath)

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

func (self *Loop4SimType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter())
}
