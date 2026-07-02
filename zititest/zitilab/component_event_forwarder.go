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
	"io/fs"
	"strings"
	"time"

	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
)

var _ model.ComponentType = (*EventForwarderType)(nil)

// EventForwarderType tails a controller's event.log and forwards matching
// events to a configurable destination. The config is rendered from a
// Go template during staging, just like router and controller configs.
type EventForwarderType struct {
	// LocalPath overrides the binary path.
	LocalPath string

	// ConfigSourceFS is the filesystem containing the config template. If nil,
	// the config template is resolved from the model's resources.Configs resource.
	ConfigSourceFS fs.FS

	// ConfigSource is the path to the config template within ConfigSourceFS.
	// Defaults to "event-forwarder.yml.tmpl".
	ConfigSource string

	// ConfigName is the rendered config file name. Defaults to "<component-id>-fwd.yml".
	ConfigName string
}

func (self *EventForwarderType) Label() string {
	return "event-forwarder"
}

func (self *EventForwarderType) GetVersion() string { return "" }
func (self *EventForwarderType) SetVersion(string)  {}
func (self *EventForwarderType) InitType(*model.Component) {}

func (self *EventForwarderType) Dump() any {
	return map[string]string{
		"type_id":    "event-forwarder",
		"local_path": self.LocalPath,
	}
}

func (self *EventForwarderType) getConfigName(c *model.Component) string {
	if self.ConfigName != "" {
		return self.ConfigName
	}
	return c.Id + "-fwd.yml"
}

func (self *EventForwarderType) StageFiles(r model.Run, c *model.Component) error {
	configSource := self.ConfigSource
	if configSource == "" {
		configSource = "event-forwarder.yml.tmpl"
	}

	configName := self.getConfigName(c)
	if err := lib.GenerateConfigForComponent(c, self.ConfigSourceFS, configSource, configName, r); err != nil {
		return err
	}

	return stageziti.StageLocalOnce(r, "event-forwarder", c, self.LocalPath)
}

func (self *EventForwarderType) getProcessFilter(c *model.Component) func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "event-forwarder") &&
			strings.Contains(s, c.Id)
	}
}

func (self *EventForwarderType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *EventForwarderType) Start(run model.Run, c *model.Component) error {
	if running, err := self.IsRunning(run, c); err != nil {
		return err
	} else if running {
		logrus.Infof("event-forwarder [%s] already running", c.Id)
		return nil
	}

	user := c.GetHost().GetSshUser()
	binaryPath := fmt.Sprintf("/home/%s/fablab/bin/event-forwarder", user)
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s", user, self.getConfigName(c))
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	serviceCmd := fmt.Sprintf("nohup %s --config %s > %s 2>&1 &",
		binaryPath, configPath, logsPath)

	logrus.Infof("starting event-forwarder [%s]: %s", c.Id, serviceCmd)
	value, err := c.GetHost().ExecLogged(serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *EventForwarderType) Stop(run model.Run, c *model.Component) error {
	if err := c.GetHost().KillProcesses("-TERM", self.getProcessFilter(c)); err != nil {
		return err
	}

	time.Sleep(2 * time.Second)

	if running, err := self.IsRunning(run, c); err != nil {
		return err
	} else if running {
		logrus.Infof("event-forwarder [%s] still running after SIGTERM, sending SIGKILL", c.Id)
		return c.GetHost().KillProcesses("-KILL", self.getProcessFilter(c))
	}

	return nil
}
