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
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"strings"
)

var _ model.ComponentType = (*ZCatType)(nil)

type ZCatMode int

type ZCatType struct {
	Version   string
	LocalPath string
}

func (self *ZCatType) Label() string {
	return "zcat"
}

func (self *ZCatType) GetVersion() string {
	return self.Version
}

func (self *ZCatType) SetVersion(version string) {
	self.Version = version
}

func (self *ZCatType) InitType(*model.Component) {
	canonicalizeGoAppVersion(&self.Version)
}

func (self *ZCatType) Dump() any {
	return map[string]string{
		"type_id":    "zcat",
		"version":    self.Version,
		"local_path": self.LocalPath,
	}
}

func (self *ZCatType) StageFiles(r model.Run, c *model.Component) error {
	return stageziti.StageZitiOnce(r, c, self.Version, self.LocalPath)
}

func (self *ZCatType) getProcessFilter() func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "ziti") && strings.Contains(s, "zcat ")
	}
}

func (self *ZCatType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter())
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *ZCatType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter())
}
