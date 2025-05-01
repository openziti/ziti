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
	"github.com/openziti/fablab/kernel/libssh"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/zititest/zitilab/cli"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
	"io/fs"
	"path/filepath"
	"strings"
)

var _ model.ComponentType = (*ZrokFrontendType)(nil)
var _ model.ServerComponent = (*ZrokFrontendType)(nil)
var _ model.FileStagingComponent = (*ZrokFrontendType)(nil)

type ZrokFrontendType struct {
	ConfigSourceFS   fs.FS
	ConfigSource     string
	ConfigName       string
	Version          string
	LocalPath        string
	DNS              string
	ZrokCtrlSelector string
}

func (self *ZrokFrontendType) Label() string {
	return "zrok-frontend"
}

func (self *ZrokFrontendType) GetVersion() string {
	return self.Version
}

func (self *ZrokFrontendType) SetVersion(version string) {
	self.Version = version
}

func (self *ZrokFrontendType) InitType(*model.Component) {
	canonicalizeGoAppVersion(&self.Version)
	if self.ZrokCtrlSelector == "" {
		self.ZrokCtrlSelector = "zrokCtrl"
	}
}

func (self *ZrokFrontendType) Dump() any {
	return map[string]string{
		"type_id":       "zrok-frontend",
		"config_source": self.ConfigSource,
		"config_name":   self.ConfigName,
		"version":       self.Version,
		"local_path":    self.LocalPath,
	}
}

func (self *ZrokFrontendType) StageFiles(r model.Run, c *model.Component) error {
	configSource := self.ConfigSource
	if configSource == "" {
		configSource = "zrok-frontend.yml.tmpl"
	}

	configName := self.getConfigName(c)

	if err := lib.GenerateConfigForComponent(c, self.ConfigSourceFS, configSource, configName, r); err != nil {
		return err
	}

	return stageziti.StageZrokOnce(r, c, self.Version, self.LocalPath)
}

func (self *ZrokFrontendType) getConfigName(c *model.Component) string {
	configName := self.ConfigName
	if configName == "" {
		configName = c.Id + ".yml"
	}
	return configName
}

func (self *ZrokFrontendType) getProcessFilter() func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "zrok") &&
			strings.Contains(s, " access public")
	}
}

func (self *ZrokFrontendType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter())
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *ZrokFrontendType) Start(_ model.Run, c *model.Component) error {
	user := c.GetHost().GetSshUser()

	binaryPath := getBinaryPath(c, constants.ZROK, self.Version)
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s", user, self.getConfigName(c))
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	serviceCmd := fmt.Sprintf("nohup %s access public %s > %s 2>&1 &", binaryPath, configPath, logsPath)

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

func (self *ZrokFrontendType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter())
}

func (self *ZrokFrontendType) Init(run model.Run, c *model.Component) error {
	id, err := cli.GetEntityId(run.GetModel(), "identities", "public")
	if err != nil {
		return err
	}

	binaryPath := getBinaryPath(c, constants.ZROK, self.Version)

	zrokSecret := run.GetModel().MustStringVariable("credentials.zrok.secret")
	zrokApiEndpoint := run.GetModel().MustSelectHost("zrokCtrl").PublicIp + ":1280"
	tmpl := "set -o pipefail; ZROK_ADMIN_TOKEN=%s ZROK_API_ENDPOINT=http://%s %s admin create frontend -- %s public http://{token}.%s:1280 2>&1 | tee logs/init.log"
	cmd := fmt.Sprintf(tmpl, zrokSecret, zrokApiEndpoint, binaryPath, id, self.DNS)
	if err = host.Exec(c.GetHost(), cmd).Execute(run); err != nil {
		return err
	}

	pfxlog.Logger().Info("fetching public frontend identity")
	zrokCtrl := run.GetModel().MustSelectHost(self.ZrokCtrlSelector)
	fullPath := fmt.Sprintf("/home/%s/.zrok/identities/public.json", zrokCtrl.GetSshUser())
	if err = libssh.RetrieveRemoteFiles(zrokCtrl.NewSshConfigFactory(), run.GetTmpDir(), fullPath); err != nil {
		return err
	}

	pfxlog.Logger().Info("sending public frontend identity")
	remoteDest := fmt.Sprintf("/home/%s/.zrok/identities/public.json", c.GetHost().GetSshUser())
	return c.GetHost().SendFile(filepath.Join(run.GetTmpDir(), "public.json"), remoteDest)
}
