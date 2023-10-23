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
	"github.com/sirupsen/logrus"
	"strings"
)

var _ model.ComponentType = (*IPerfServerType)(nil)

type IPerfServerType struct {
	Port uint16
}

func (self *IPerfServerType) Dump() any {
	return map[string]string{
		"type_id": "iperf-server",
		"port":    fmt.Sprintf("%v", self.GetPort()),
	}
}

func (self *IPerfServerType) GetPort() uint16 {
	if self.Port == 0 {
		return 5201
	}
	return self.Port
}

func (self *IPerfServerType) getProcessFilter() func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, fmt.Sprintf("iperf3 -s -p %v", self.GetPort()))
	}
}

func (self *IPerfServerType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter())
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *IPerfServerType) Start(_ model.Run, c *model.Component) error {
	user := c.GetHost().GetSshUser()
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)
	serviceCmd := fmt.Sprintf("nohup iperf3 -s -p %v > %s 2>&1 &", self.GetPort(), logsPath)

	value, err := c.GetHost().ExecLogged(serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *IPerfServerType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter())
}
