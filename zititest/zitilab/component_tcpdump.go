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

var _ model.ComponentType = (*TcpDumpType)(nil)

type TcpDumpType struct {
	Filter          string
	MaxFileSizeInMb uint32
}

func (self *TcpDumpType) Label() string {
	return "tcpdump"
}

func (self *TcpDumpType) GetVersion() string {
	return "os-provided"
}

func (self *TcpDumpType) Dump() any {
	return map[string]string{
		"type_id": "tcpdump",
	}
}

func (self *TcpDumpType) getBinaryName() string {
	return "tcpdump"
}

func (self *TcpDumpType) getProcessFilter(c *model.Component) func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, self.getBinaryName()) &&
			strings.Contains(s, c.Id) &&
			strings.Contains(s, self.Filter)
	}
}

func (self *TcpDumpType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *TcpDumpType) Start(_ model.Run, c *model.Component) error {
	user := c.GetHost().GetSshUser()

	capturePath := fmt.Sprintf("/home/%s/logs/%s.pcap", user, c.Id)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	maxFileSize := ""
	if self.MaxFileSizeInMb > 0 {
		maxFileSize = fmt.Sprintf("-C %d ", self.MaxFileSizeInMb)
	}

	serviceCmd := fmt.Sprintf("sudo tcpdump -Z %s -s 64 -W 10 %s -w %s %s > %s 2>&1 &", user, maxFileSize, capturePath, self.Filter, logsPath)
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

func (self *TcpDumpType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter(c))
}
