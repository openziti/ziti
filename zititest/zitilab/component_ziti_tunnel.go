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

var _ model.ComponentType = (*ZitiTunnelType)(nil)

type ZitiTunnelMode int

const (
	ZitiTunnelModeTproxy ZitiTunnelMode = 0
	ZitiTunnelModeProxy  ZitiTunnelMode = 1
	ZitiTunnelModeHost   ZitiTunnelMode = 2

	ZitiTunnelActionsReEnroll = "reEnroll"
)

func (self ZitiTunnelMode) String() string {
	if self == ZitiTunnelModeTproxy {
		return "tproxy"
	}
	if self == ZitiTunnelModeProxy {
		return "proxy"
	}
	if self == ZitiTunnelModeHost {
		return "host"
	}
	return "invalid"
}

type ZitiTunnelType struct {
	Mode        ZitiTunnelMode
	Version     string
	LocalPath   string
	ConfigPathF func(c *model.Component) string
}

func (self *ZitiTunnelType) GetActions() map[string]model.ComponentAction {
	return map[string]model.ComponentAction{
		ZitiTunnelActionsReEnroll: model.ComponentActionF(self.ReEnroll),
	}
}

func (self *ZitiTunnelType) InitType(*model.Component) {
	canonicalizeZitiVersion(&self.Version)
}

func (self *ZitiTunnelType) Dump() any {
	return map[string]string{
		"type_id":    "ziti-tunnel",
		"version":    self.Version,
		"local_path": self.LocalPath,
	}
}

func (self *ZitiTunnelType) StageFiles(r model.Run, c *model.Component) error {
	return stageziti.StageZitiOnce(r, c, self.Version, self.LocalPath)
}

func (self *ZitiTunnelType) InitializeHost(_ model.Run, c *model.Component) error {
	if self.Mode == ZitiTunnelModeTproxy {
		cmds := []string{
			"sudo sed -i 's/#DNS=/DNS=127.0.0.1/g' /etc/systemd/resolved.conf",
			"sudo systemctl restart systemd-resolved",
		}
		return c.Host.ExecLogOnlyOnError(cmds...)
	}
	return nil
}

func (self *ZitiTunnelType) getProcessFilter(c *model.Component) func(string) bool {
	return getZitiProcessFilter(c, "tunnel")
}

func (self *ZitiTunnelType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *ZitiTunnelType) GetConfigPath(c *model.Component) string {
	if self.ConfigPathF != nil {
		return self.ConfigPathF(c)
	}
	return fmt.Sprintf("/home/%s/fablab/cfg/%s.json", c.GetHost().GetSshUser(), c.Id)
}

func (self *ZitiTunnelType) Start(_ model.Run, c *model.Component) error {
	mode := self.Mode

	user := c.GetHost().GetSshUser()

	binaryPath := getZitiBinaryPath(c, self.Version)
	configPath := self.GetConfigPath(c)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	useSudo := ""
	if mode == ZitiTunnelModeTproxy {
		useSudo = "sudo"
	}

	serviceCmd := fmt.Sprintf("%s %s tunnel %s -v --log-formatter pfxlog -i %s --cli-agent-alias %s > %s 2>&1 &",
		useSudo, binaryPath, mode.String(), configPath, c.Id, logsPath)

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

func (self *ZitiTunnelType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-KILL", self.getProcessFilter(c))
}

func (self *ZitiTunnelType) ReEnroll(run model.Run, c *model.Component) error {
	return reEnrollIdentity(run, c, getZitiBinaryPath(c, self.Version), self.GetConfigPath(c))
}
