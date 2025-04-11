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
	"github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
	"io/fs"
	"strings"
)

var _ model.ComponentType = (*CaddyType)(nil)
var _ model.ServerComponent = (*CaddyType)(nil)
var _ model.FileStagingComponent = (*CaddyType)(nil)
var _ model.HostInitializingComponent = (*CaddyType)(nil)

type CaddyType struct {
	ConfigSourceFS   fs.FS
	ConfigSource     string
	ConfigName       string
	Version          string
	LocalPath        string
	PreCreateClients string
}

func (self *CaddyType) InitializeHost(r model.Run, c *model.Component) error {
	return c.GetHost().ExecLogOnlyOnError("cd www && tar xfjv files.tar.bz2")
}

func (self *CaddyType) Label() string {
	return "caddy"
}

func (self *CaddyType) GetVersion() string {
	return self.Version
}

func (self *CaddyType) SetVersion(version string) {
	self.Version = version
}

func (self *CaddyType) InitType(*model.Component) {
	canonicalizeGoAppVersion(&self.Version)
}

func (self *CaddyType) Dump() any {
	return map[string]string{
		"type_id":       "caddy",
		"config_source": self.ConfigSource,
		"config_name":   self.ConfigName,
		"version":       self.Version,
		"local_path":    self.LocalPath,
	}
}

func (self *CaddyType) StageFiles(r model.Run, c *model.Component) error {
	configSource := self.ConfigSource
	if configSource == "" {
		configSource = "Caddyfile.tmpl"
	}

	configName := self.getConfigName(c)

	if err := lib.GenerateConfigForComponent(c, self.ConfigSourceFS, configSource, configName, r); err != nil {
		return err
	}

	return stageziti.StageCaddy(r, c, self.Version, self.LocalPath)
}

func (self *CaddyType) getConfigName(c *model.Component) string {
	configName := self.ConfigName
	if configName == "" {
		configName = c.Id
	}
	return configName
}

func (self *CaddyType) getProcessFilter() func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "caddy")
	}
}

func (self *CaddyType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter())
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *CaddyType) Start(_ model.Run, c *model.Component) error {
	binaryPath := getBinaryPath(c, constants.Caddy, self.Version)
	configPath := self.getConfigPath(c)

	user := c.GetHost().GetSshUser()

	logsPath := fmt.Sprintf("/home/%s/logs/%s-ctrl.log", user, c.Id)
	serviceCmd := fmt.Sprintf("%s start --adapter caddyfile --config %s >> %s 2>&1 &", binaryPath, configPath, logsPath)

	if quiet, _ := c.GetBoolVariable("quiet_startup"); !quiet {
		logrus.Info(serviceCmd)
	}

	value, err := c.GetHost().ExecLogged(serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *CaddyType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter())
}

func (self *CaddyType) getConfigPath(c *model.Component) string {
	return fmt.Sprintf("/home/%s/fablab/cfg/%s", c.GetHost().GetSshUser(), self.getConfigName(c))
}
