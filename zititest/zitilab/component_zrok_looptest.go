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
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
)

var _ model.ComponentType = (*ZrokLoopTestType)(nil)
var _ model.ServerComponent = (*ZrokLoopTestType)(nil)
var _ model.FileStagingComponent = (*ZrokLoopTestType)(nil)

type ZrokLoopTestType struct {
	Version    string
	LocalPath  string
	Iterations uint32
	Loopers    uint8
	Pacing     time.Duration
}

func (self *ZrokLoopTestType) InitType(*model.Component) {
	canonicalizeGoAppVersion(&self.Version)
	if self.Iterations == 0 {
		self.Iterations = 1
	}
	if self.Loopers == 0 {
		self.Loopers = 1
	}
}

func (self *ZrokLoopTestType) Dump() any {
	return map[string]string{
		"type_id":    "zrok-test-loop",
		"version":    self.Version,
		"local_path": self.LocalPath,
	}
}

func (self *ZrokLoopTestType) StageFiles(r model.Run, c *model.Component) error {
	return stageziti.StageZrokOnce(r, c, self.Version, self.LocalPath)
}

func (self *ZrokLoopTestType) getProcessFilter() func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "zrok") &&
			strings.Contains(s, " test loop public") &&
			!strings.Contains(s, "sudo")
	}
}

func (self *ZrokLoopTestType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter())
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *ZrokLoopTestType) Start(_ model.Run, c *model.Component) error {
	user := c.GetHost().GetSshUser()
	userId := self.getUnixUser(c)

	binaryPath := getBinaryPath(c, constants.ZROK, self.Version)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	serviceCmd := fmt.Sprintf("nohup sudo -u %s %s test loop public --iterations %v --loopers %v --min-pacing-ms %v --max-pacing-ms %v 2>&1 &> %s &",
		userId, binaryPath, self.Iterations, self.Loopers, self.Pacing.Milliseconds(), self.Pacing.Milliseconds(), logsPath)

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

func (self *ZrokLoopTestType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter())
}

func (self *ZrokLoopTestType) getUnixUser(c *model.Component) string {
	return fmt.Sprintf("zrok%v", c.ScaleIndex)
}

func (self *ZrokLoopTestType) InitializeHost(_ model.Run, c *model.Component) error {
	userId := self.getUnixUser(c)

	if _, err := c.GetHost().ExecLogged(fmt.Sprintf("id -u %s", userId)); err != nil {
		cmd := fmt.Sprintf("sudo useradd %s -m -g ubuntu ", userId)
		pfxlog.Logger().Info(cmd)
		if err = c.GetHost().ExecLogOnlyOnError(fmt.Sprintf("sudo useradd %s -m -g ubuntu ", userId)); err != nil {
			return err
		}
	}
	return nil
}

func (self *ZrokLoopTestType) Init(run model.Run, c *model.Component) error {
	userId := self.getUnixUser(c)

	binaryPath := getBinaryPath(c, constants.ZROK, self.Version)
	val, ok := c.Data["token"]
	if !ok {
		return fmt.Errorf("no token found for zrok client '%s'", c.Id)
	}
	token := fmt.Sprintf("%v", val)
	zrokApiEndpoint := run.GetModel().MustSelectHost("zrokCtrl").PublicIp + ":1280"
	tmpl := "set -o pipefail; sudo -u %s rm -rf /home/%s/.zrok && sudo -u %s ZROK_API_ENDPOINT=http://%s %s enable %s"
	cmd := fmt.Sprintf(tmpl, userId, userId, userId, zrokApiEndpoint, binaryPath, token)
	pfxlog.Logger().Info(cmd)
	return c.GetHost().ExecLogOnlyOnError(cmd)
}
