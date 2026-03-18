/*
	Copyright NetFoundry Inc.

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
	"strings"

	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
)

var _ model.ComponentType = (*IRTTServerType)(nil)

// IRTTServerType is a component type that runs an IRTT (Isochronous Round-Trip Tester)
// server. IRTT measures round-trip time and one-way delay using UDP packets sent on a
// fixed period. The irtt package must be installed on the host (e.g. via apt).
type IRTTServerType struct {
	Port    uint16
	Install bool
}

func (self *IRTTServerType) Label() string {
	return "irtt-server"
}

func (self *IRTTServerType) GetVersion() string {
	return "os-provided"
}

func (self *IRTTServerType) InitializeHost(r model.Run, c *model.Component) error {
	if self.Install {
		return c.Host.ExecLogOnlyOnError("sudo apt-get update -qq && sudo apt-get install -y -qq irtt")
	}
	return nil
}

func (self *IRTTServerType) Dump() any {
	return map[string]string{
		"type_id": "irtt-server",
		"port":    fmt.Sprintf("%v", self.GetPort()),
	}
}

// GetPort returns the configured port, defaulting to 2112 (IRTT's default).
func (self *IRTTServerType) GetPort() uint16 {
	if self.Port == 0 {
		return 2112
	}
	return self.Port
}

func (self *IRTTServerType) getProcessFilter() func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, fmt.Sprintf("irtt server -b :%v", self.GetPort()))
	}
}

func (self *IRTTServerType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter())
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *IRTTServerType) Start(_ model.Run, c *model.Component) error {
	user := c.GetHost().GetSshUser()
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)
	serviceCmd := fmt.Sprintf("nohup irtt server -b :%v > %s 2>&1 &", self.GetPort(), logsPath)

	value, err := c.GetHost().ExecLogged(serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *IRTTServerType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter())
}
