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
	"strings"

	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
)

var _ model.ComponentType = (*ZitiEdgeTunnelType)(nil)

type ZitiEdgeTunnelMode int

const (
	ZitiEdgeTunnelModeDefault ZitiEdgeTunnelMode = 0
	ZitiEdgeTunnelModeHost    ZitiEdgeTunnelMode = 1
)

type ZitiEdgeTunnelType struct {
	Mode        ZitiEdgeTunnelMode
	Version     string
	ZitiVersion string
	LocalPath   string
	// LogVerbosity is the ziti-edge-tunnel ZITI_LOG spec (e.g. "2;bind.c=6"); empty leaves it unset.
	LogVerbosity   string
	VerbosityLevel uint16
	ConfigPathF    func(c *model.Component) string
	LogConfig
}

func (self *ZitiEdgeTunnelType) Label() string {
	return "ziti-edge-tunnel"
}

func (self *ZitiEdgeTunnelType) GetVersion() string {
	return self.Version
}

func (self *ZitiEdgeTunnelType) SetVersion(version string) {
	self.Version = version
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
	canonicalizeGoAppVersion(&self.ZitiVersion)
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
	// LocalPath is the ziti-edge-tunnel build, not a ziti build, so it must not be reused as the
	// ziti source: with an empty ZitiVersion that would stage the ZET executable as "ziti" and break
	// rotation's "ziti ops log-pipe". An empty source resolves ziti from ZITI_PATH/PATH instead.
	if err := stageziti.StageZitiOnce(r, c, self.ZitiVersion, ""); err != nil {
		return err
	}
	// The rotate log strategy runs "ops log-pipe" from the staged ziti binary (ZitiVersion);
	// stage a different one when LogConfig.PipeBinaryVersion overrides it.
	return stageLogPipeBinary(r, c, self.ZitiVersion)
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

func (self *ZitiEdgeTunnelType) Start(r model.Run, c *model.Component) error {
	isRunninng, err := self.IsRunning(r, c)
	if err != nil {
		return err
	}

	if isRunninng {
		fmt.Printf("ziti-edge-tunnel %s already started\n", c.Id)
		return nil
	}

	user := c.GetHost().GetSshUser()

	binaryPath := fmt.Sprintf("/home/%s/fablab/bin/%s", user, self.getBinaryName())
	configPath := self.GetConfigPath(c)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	env := "ZITI_TIME_FORMAT=utc"
	if self.LogVerbosity != "" {
		env += " ZITI_LOG=" + self.LogVerbosity
	}

	verbosity := ""
	if self.VerbosityLevel > 0 {
		verbosity = fmt.Sprintf("-v %v", self.VerbosityLevel)
	}

	// ziti-edge-tunnel has no "ops log-pipe" command, so rotation must run it from the ziti
	// binary staged alongside (ZitiVersion), not from binaryPath (the ziti-edge-tunnel binary).
	redirect := logRedirect(c, logsPath, GetZitiBinaryPath(c, self.ZitiVersion))

	var serviceCmd string
	if self.Mode == ZitiEdgeTunnelModeDefault {
		serviceCmd = fmt.Sprintf("%s sudo -E %s run -i %s %s %s &", env, binaryPath, configPath, verbosity, redirect)
	} else if self.Mode == ZitiEdgeTunnelModeHost {
		serviceCmd = fmt.Sprintf("%s %s run-host -i %s %s %s &", env, binaryPath, configPath, verbosity, redirect)
	} else {
		return fmt.Errorf("unsupported ziti-edge-tunnel mode: %v", self.Mode)
	}

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
	return reEnrollIdentity(run, c, GetZitiBinaryPath(c, self.ZitiVersion), self.GetConfigPath(c))
}
