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
	Mode      ZitiTunnelMode
	Version   string
	LocalPath string
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

func (self *ZitiTunnelType) InitializeHost(run model.Run, c *model.Component) error {
	cmds := []string{"mkdir -p /home/ubuntu/logs"}
	if self.Mode == ZitiTunnelModeTproxy {
		cmds = append(cmds,
			"sudo sed -i 's/#DNS=/DNS=127.0.0.1/g' /etc/systemd/resolved.conf",
			"sudo systemctl restart systemd-resolved",
		)
	}
	return host.Exec(c.GetHost(), cmds...).Execute(run)
}

func (self *ZitiTunnelType) getProcessFilter(c *model.Component) func(string) bool {
	return getZitiProcessFilter(c, "tunnel")
}

func (self *ZitiTunnelType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	factory := lib.NewSshConfigFactory(c.GetHost())
	pids, err := lib.FindProcesses(factory, self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *ZitiTunnelType) Start(_ model.Run, c *model.Component) error {
	binaryName := "ziti"
	if self.Version != "" {
		binaryName += "-" + self.Version
	}

	mode := self.Mode

	factory := lib.NewSshConfigFactory(c.GetHost())

	binaryPath := fmt.Sprintf("/home/%s/fablab/bin/%s", factory.User(), binaryName)
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s.json", factory.User(), c.Id)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", factory.User(), c.Id)

	useSudo := ""
	if mode == ZitiTunnelModeTproxy {
		useSudo = "sudo"
	}

	serviceCmd := fmt.Sprintf("nohup %s %s tunnel %s --log-formatter pfxlog -i %s --cli-agent-alias %s > %s 2>&1 &",
		useSudo, binaryPath, mode.String(), configPath, c.Id, logsPath)

	value, err := lib.RemoteExec(factory, serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *ZitiTunnelType) Stop(_ model.Run, c *model.Component) error {
	factory := lib.NewSshConfigFactory(c.GetHost())
	return lib.RemoteKillSignalFilterF(factory, "-KILL", self.getProcessFilter(c))
}
