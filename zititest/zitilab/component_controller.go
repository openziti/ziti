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
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/pki"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/pkg/errors"
	"io/fs"
)

var _ model.ComponentType = (*ControllerType)(nil)
var _ model.ServerComponent = (*ControllerType)(nil)
var _ model.FileStagingComponent = (*ControllerType)(nil)
var _ model.ActionsComponent = (*ControllerType)(nil)

const (
	ControllerActionInitStandalone = "initStandalone"
)

type ControllerType struct {
	ConfigSourceFS fs.FS
	ConfigSource   string
	ConfigName     string
	Version        string
	LocalPath      string
	DNSNames       []string
	Debug          bool
}

func (self *ControllerType) Label() string {
	return "ziti-controller"
}

func (self *ControllerType) GetVersion() string {
	return self.Version
}

func (self *ControllerType) SetVersion(version string) {
	self.Version = version
}

func (self *ControllerType) InitType(*model.Component) {
	canonicalizeGoAppVersion(&self.Version)
}

func (self *ControllerType) GetActions() map[string]model.ComponentAction {
	return map[string]model.ComponentAction{
		ControllerActionInitStandalone: model.ComponentActionF(self.InitStandalone),
	}
}

func (self *ControllerType) Dump() any {
	return map[string]string{
		"type_id":       "controller",
		"config_source": self.ConfigSource,
		"config_name":   self.ConfigName,
		"version":       self.Version,
		"local_path":    self.LocalPath,
	}
}

func (self *ControllerType) StageFiles(r model.Run, c *model.Component) error {
	configSource := self.ConfigSource
	if configSource == "" {
		configSource = "ctrl.yml.tmpl"
	}

	configName := self.getConfigName(c)

	if err := lib.GenerateConfigForComponent(c, self.ConfigSourceFS, configSource, configName, r); err != nil {
		return err
	}

	if err := pki.CreateControllerCerts(r, c, self.DNSNames, c.Id); err != nil {
		return err
	}

	return stageziti.StageZitiOnce(r, c, self.Version, self.LocalPath)
}

func (self *ControllerType) getConfigName(c *model.Component) string {
	configName := self.ConfigName
	if configName == "" {
		configName = c.Id + ".yml"
	}
	return configName
}

func (self *ControllerType) getProcessFilter(c *model.Component) func(string) bool {
	return getZitiProcessFilter(c, "controller")
}

func (self *ControllerType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *ControllerType) Start(r model.Run, c *model.Component) error {
	isRunninng, err := self.IsRunning(r, c)
	if err != nil {
		return err
	}
	if isRunninng {
		fmt.Printf("controller %s already started\n", c.Id)
		return nil
	}
	extraArgs := ""
	if self.Debug {
		extraArgs = " -v "
	}
	return startZitiComponent(c, "controller", self.Version, self.getConfigName(c), extraArgs)
}

func (self *ControllerType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter(c))
}

func (self *ControllerType) GetBinaryPath(c *model.Component) string {
	return GetZitiBinaryPath(c, self.Version)
}

func (self *ControllerType) InitStandalone(run model.Run, c *model.Component) error {
	username := c.MustStringVariable("credentials.edge.username")
	password := c.MustStringVariable("credentials.edge.password")

	if username == "" {
		return errors.New("variable credentials/edge/username must be a string")
	}

	if password == "" {
		return errors.New("variable credentials/edge/password must be a string")
	}

	factory := c.GetHost().NewSshConfigFactory()

	binaryPath := GetZitiBinaryPath(c, self.Version)
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s", factory.User(), self.getConfigName(c))

	tmpl := "rm -f /home/%v/fablab/ctrl.db && set -o pipefail; %s controller --log-formatter pfxlog edge init %s -u %s -p %s 2>&1 | tee logs/controller.edge.init.log"
	cmd := fmt.Sprintf(tmpl, factory.User(), binaryPath, configPath, username, password)
	return host.Exec(c.GetHost(), cmd).Execute(run)
}

func (self *ControllerType) InitRaft(run model.Run, c *model.Component) error {
	count := 0
	err := run.GetModel().ForEachComponent("*", 1, func(other *model.Component) error {
		if _, ok := c.Type.(*ControllerType); ok {
			binaryPath := GetZitiBinaryPath(c, self.Version)
			tmpl := "%s agent cluster add %v --id %v"
			cmd := fmt.Sprintf(tmpl, binaryPath, "tls:"+other.Host.PublicIp+":6262", other.Id)
			count++
			return c.GetHost().ExecLogOnlyOnError(cmd)
		}
		return nil
	})

	if err != nil {
		return err
	}

	if count < 1 {
		return errors.New("no other controllers found")
	}

	return nil
}
