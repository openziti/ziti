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

var _ model.ComponentType = (*ZitiEdgeTunnelType)(nil)

type ZitiEdgeTunnelType struct {
	Version     string
	ZitiVersion string
	LocalPath   string
	LogConfig   string
	ConfigPathF func(c *model.Component) string
}

func (self *ZitiEdgeTunnelType) GetActions() map[string]model.ComponentAction {
	return map[string]model.ComponentAction{
		ZitiTunnelActionsReEnroll: model.ComponentActionF(self.ReEnroll),
	}
}

func (self *ZitiEdgeTunnelType) Dump() any {
	return map[string]string{
		"type_id":    "ziti-edge-tunnel",
		"version":    self.Version,
		"local_path": self.LocalPath,
	}
}

func (self *ZitiEdgeTunnelType) InitType(*model.Component) {
	if strings.HasPrefix(self.Version, "v") {
		self.Version = self.Version[1:]
	}
	canonicalizeZitiVersion(&self.ZitiVersion)
}

func (self *ZitiEdgeTunnelType) getBinaryName() string {
	binaryName := "ziti-edge-tunnel"
	version := self.Version
	if version != "" {
		binaryName += "-" + version
	}
	return binaryName
}

func (self *ZitiEdgeTunnelType) StageFiles(r model.Run, c *model.Component) error {
	if err := stageziti.StageZitiEdgeTunnelOnce(r, c, self.Version, self.LocalPath); err != nil {
		return err
	}
	return stageziti.StageZitiOnce(r, c, self.ZitiVersion, self.LocalPath)
}

func (self *ZitiEdgeTunnelType) getProcessFilter(c *model.Component) func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, self.getBinaryName()) &&
			strings.Contains(s, fmt.Sprintf("%s.json", c.Id)) &&
			!strings.Contains(s, "sudo ")
	}
}

func (self *ZitiEdgeTunnelType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *ZitiEdgeTunnelType) GetConfigPath(c *model.Component) string {
	if self.ConfigPathF != nil {
		return self.ConfigPathF(c)
	}
	return fmt.Sprintf("/home/%s/fablab/cfg/%s.json", c.GetHost().GetSshUser(), c.Id)
}

func (self *ZitiEdgeTunnelType) Start(_ model.Run, c *model.Component) error {
	user := c.GetHost().GetSshUser()

	binaryPath := fmt.Sprintf("/home/%s/fablab/bin/%s", user, self.getBinaryName())
	configPath := self.GetConfigPath(c)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	logging := ""
	if self.LogConfig != "" {
		logging = "ZITI_LOG=" + self.LogConfig + " "
	}

	serviceCmd := fmt.Sprintf("%ssudo %s run -i %s > %s 2>&1 &", logging, binaryPath, configPath, logsPath)
	logrus.Infof("starting: %s", serviceCmd)
	value, err := c.GetHost().ExecLogged(serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *ZitiEdgeTunnelType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter(c))
}

func (self *ZitiEdgeTunnelType) ReEnroll(run model.Run, c *model.Component) error {
	return reEnrollIdentity(run, c, getZitiBinaryPath(c, self.ZitiVersion), self.GetConfigPath(c))
}
