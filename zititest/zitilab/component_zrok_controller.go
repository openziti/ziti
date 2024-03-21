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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
	"io/fs"
	"strings"
)

var _ model.ComponentType = (*ZrokControllerType)(nil)
var _ model.ServerComponent = (*ZrokControllerType)(nil)
var _ model.FileStagingComponent = (*ZrokControllerType)(nil)
var _ model.ActionsComponent = (*ZrokControllerType)(nil)

const (
	ZrokControllerActionPreCreateAccounts = "preCreateAccounts"
)

type ZrokControllerType struct {
	ConfigSourceFS   fs.FS
	ConfigSource     string
	ConfigName       string
	Version          string
	LocalPath        string
	PreCreateClients string
}

func (self *ZrokControllerType) Label() string {
	return "zrok-controller"
}

func (self *ZrokControllerType) GetVersion() string {
	return self.Version
}

func (self *ZrokControllerType) InitType(*model.Component) {
	canonicalizeGoAppVersion(&self.Version)
}

func (self *ZrokControllerType) GetActions() map[string]model.ComponentAction {
	return map[string]model.ComponentAction{
		ZrokControllerActionPreCreateAccounts: model.ComponentActionF(self.PreCreateAccounts),
	}
}

func (self *ZrokControllerType) Dump() any {
	return map[string]string{
		"type_id":       "zrok-controller",
		"config_source": self.ConfigSource,
		"config_name":   self.ConfigName,
		"version":       self.Version,
		"local_path":    self.LocalPath,
	}
}

func (self *ZrokControllerType) StageFiles(r model.Run, c *model.Component) error {
	configSource := self.ConfigSource
	if configSource == "" {
		configSource = "zrok.yml.tmpl"
	}

	configName := self.getConfigName(c)

	if err := lib.GenerateConfigForComponent(c, self.ConfigSourceFS, configSource, configName, r); err != nil {
		return err
	}

	return stageziti.StageZrokOnce(r, c, self.Version, self.LocalPath)
}

func (self *ZrokControllerType) getConfigName(c *model.Component) string {
	configName := self.ConfigName
	if configName == "" {
		configName = c.Id + ".yml"
	}
	return configName
}

func (self *ZrokControllerType) getProcessFilter() func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "zrok") &&
			strings.Contains(s, " controller")
	}
}

func (self *ZrokControllerType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter())
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *ZrokControllerType) Start(_ model.Run, c *model.Component) error {
	user := c.GetHost().GetSshUser()

	binaryPath := getBinaryPath(c, constants.ZROK, self.Version)
	configPath := self.getConfigPath(c)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	serviceCmd := fmt.Sprintf("nohup %s controller %s > %s 2>&1 &", binaryPath, configPath, logsPath)

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

func (self *ZrokControllerType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter())
}

func (self *ZrokControllerType) getConfigPath(c *model.Component) string {
	return fmt.Sprintf("/home/%s/fablab/cfg/%s", c.GetHost().GetSshUser(), self.getConfigName(c))
}

func (self *ZrokControllerType) Init(run model.Run, c *model.Component) error {
	binaryPath := getBinaryPath(c, constants.ZROK, self.Version)
	configPath := self.getConfigPath(c)

	tmpl := "rm -f /home/%v/zrok.db && set -o pipefail; %s admin bootstrap %s 2>&1 | tee logs/init.zrok.log"
	cmd := fmt.Sprintf(tmpl, c.GetHost().GetSshUser(), binaryPath, configPath)
	return host.Exec(c.GetHost(), cmd).Execute(run)
}

func (self *ZrokControllerType) PreCreateAccounts(run model.Run, c *model.Component) error {
	binaryPath := getBinaryPath(c, constants.ZROK, self.Version)
	configPath := self.getConfigPath(c)

	components := run.GetModel().SelectComponents(self.PreCreateClients)
	if len(components) == 0 {
		return fmt.Errorf("found no zrok clients for component spec '%s'", self.PreCreateClients)
	}
	for _, clientComponent := range components {
		log := pfxlog.Logger().WithField("id", clientComponent.Id)

		tmpl := "%s admin create account %s -- %s@openziti.org %s 2>&1"
		cmd := fmt.Sprintf(tmpl, binaryPath, configPath, clientComponent.Id, clientComponent.Id)
		log.Info(cmd)
		output, err := c.GetHost().ExecLogged(cmd)
		if err != nil {
			log.WithError(err).WithField("output", output).Error("error creating account")
			return err
		}

		parts := strings.Split(output, "token = ")
		if len(parts) != 2 {
			return fmt.Errorf("unable to parse output for token: %s", output)
		}
		token := parts[1]
		token = token[:strings.Index(token, `"`)]

		clientComponent.Data["token"] = token
		log.WithField("token", token).Info("client created")
	}
	return nil
}
