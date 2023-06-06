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
	"strings"
)

var _ model.ComponentType = (*ZitiEdgeTunnelType)(nil)

type ZitiEdgeTunnelType struct {
	Version   string
	LocalPath string
}

func (self *ZitiEdgeTunnelType) Dump() any {
	return map[string]string{
		"type_id":    "ziti-edge-tunnel",
		"version":    self.Version,
		"local_path": self.LocalPath,
	}
}

func (self *ZitiEdgeTunnelType) getVersion() string {
	if strings.HasPrefix(self.Version, "v") {
		return self.Version[1:]
	}
	return self.Version
}

func (self *ZitiEdgeTunnelType) getBinaryName() string {
	binaryName := "ziti-edge-tunnel"
	version := self.getVersion()
	if version != "" {
		binaryName += "-" + version
	}
	return binaryName
}

func (self *ZitiEdgeTunnelType) StageFiles(r model.Run, c *model.Component) error {
	return stageziti.StageZitiEdgeTunnelOnce(r, c, self.getVersion(), self.LocalPath)
}

func (self *ZitiEdgeTunnelType) getProcessFilter(c *model.Component) func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, self.getBinaryName()) &&
			strings.Contains(s, fmt.Sprintf("%s.json", c.Id)) &&
			!strings.Contains(s, "sudo ")
	}
}

func (self *ZitiEdgeTunnelType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	factory := lib.NewSshConfigFactory(c.GetHost())
	pids, err := lib.FindProcesses(factory, self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *ZitiEdgeTunnelType) Start(_ model.Run, c *model.Component) error {
	factory := lib.NewSshConfigFactory(c.GetHost())

	binaryPath := fmt.Sprintf("/home/%s/fablab/bin/%s", factory.User(), self.getBinaryName())
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s.json", factory.User(), c.Id)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", factory.User(), c.Id)

	serviceCmd := fmt.Sprintf("nohup sudo %s run -i %s > %s 2>&1 &", binaryPath, configPath, logsPath)

	value, err := lib.RemoteExec(factory, serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *ZitiEdgeTunnelType) Stop(_ model.Run, c *model.Component) error {
	factory := lib.NewSshConfigFactory(c.GetHost())
	return lib.RemoteKillFilterF(factory, self.getProcessFilter(c))
}
