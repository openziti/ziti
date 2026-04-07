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
	"strconv"
	"strings"
	"time"

	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
)

var _ model.ComponentType = (*ZitiProxCType)(nil)

// ZitiProxCType is a component type for the C-SDK based ziti-prox-c proxy application.
// It runs as an unprivileged process, authenticates to the overlay via OIDC using its
// identity, and proxies connections to Ziti services on local ports.
//
// When Services and BasePort are configured, each instance listens for each service
// on a deterministic port: BasePort + instanceIndex * len(Services) + serviceIndex.
// This allows a single traffic driver per host to rotate through all proxy instances.
type ZitiProxCType struct {
	// Version is the ziti-prox-c release version (without "v" prefix).
	Version string

	// ZitiVersion is the version of the ziti binary used for enrollment.
	ZitiVersion string

	// LocalPath overrides the binary staging path.
	LocalPath string

	// Services is a list of Ziti service names to proxy (e.g., ["svc-ert", "svc-go", "svc-zde"]).
	// Each service gets a unique listener port per instance.
	Services []string

	// BasePort is the starting port number for listener allocation. Instance I, service S
	// listens on BasePort + I * len(Services) + S. Defaults to 10000.
	BasePort int

	// ConfigPathF optionally overrides the identity config path.
	ConfigPathF func(c *model.Component) string

	// LogLevel sets the C-SDK log level (0=NONE, 1=ERROR, 2=WARN, 3=INFO,
	// 4=DEBUG, 5=VERBOSE, 6=TRACE). Defaults to 2 (WARN).
	LogLevel int
}

func (self *ZitiProxCType) Label() string {
	return "ziti-prox-c"
}

func (self *ZitiProxCType) GetVersion() string {
	return self.Version
}

func (self *ZitiProxCType) SetVersion(version string) {
	self.Version = version
}

func (self *ZitiProxCType) InitType(*model.Component) {
	if strings.HasPrefix(self.Version, "v") {
		self.Version = self.Version[1:]
	}
	canonicalizeGoAppVersion(&self.ZitiVersion)
	if self.BasePort == 0 {
		self.BasePort = 10000
	}
}

func (self *ZitiProxCType) Dump() any {
	return map[string]string{
		"type_id":    "ziti-prox-c",
		"version":    self.Version,
		"local_path": self.LocalPath,
	}
}

func (self *ZitiProxCType) getBinaryName() string {
	name := "ziti-prox-c"
	if self.Version != "" {
		name += "-" + self.Version
	}
	return name
}

func (self *ZitiProxCType) StageFiles(r model.Run, c *model.Component) error {
	if err := stageziti.StageZitiProxCOnce(r, c, self.Version, self.LocalPath); err != nil {
		return err
	}
	return stageziti.StageZitiOnce(r, c, self.ZitiVersion, self.LocalPath)
}

func (self *ZitiProxCType) GetConfigPath(c *model.Component) string {
	if self.ConfigPathF != nil {
		return self.ConfigPathF(c)
	}
	return fmt.Sprintf("/home/%s/fablab/cfg/%s.json", c.GetHost().GetSshUser(), c.Id)
}

func (self *ZitiProxCType) getProcessFilter(c *model.Component) func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, self.getBinaryName()) &&
			strings.Contains(s, fmt.Sprintf("%s.json", c.Id))
	}
}

func (self *ZitiProxCType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

// parseInstanceIndex extracts the trailing numeric index from a component ID.
// For example, "prox-us-3-42" returns 42.
func parseInstanceIndex(id string) (int, error) {
	parts := strings.Split(id, "-")
	if len(parts) == 0 {
		return 0, fmt.Errorf("cannot parse instance index from empty id")
	}
	return strconv.Atoi(parts[len(parts)-1])
}

func (self *ZitiProxCType) Start(run model.Run, c *model.Component) error {
	if running, err := self.IsRunning(run, c); err != nil {
		return err
	} else if running {
		logrus.Infof("ziti-prox-c [%s] already running", c.Id)
		return nil
	}

	user := c.GetHost().GetSshUser()

	binaryPath := fmt.Sprintf("/home/%s/fablab/bin/%s", user, self.getBinaryName())
	configPath := self.GetConfigPath(c)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	// Build listener args: <serviceName>:<port> for each service.
	var listeners string
	if len(self.Services) > 0 {
		idx, err := parseInstanceIndex(c.Id)
		if err != nil {
			return fmt.Errorf("failed to parse instance index from %q: %w", c.Id, err)
		}
		var parts []string
		for svcIdx, svc := range self.Services {
			port := self.BasePort + idx*len(self.Services) + svcIdx
			parts = append(parts, fmt.Sprintf("%s:%d", svc, port))
		}
		listeners = " " + strings.Join(parts, " ")
	}

	debugFlag := ""
	if self.LogLevel > 0 {
		debugFlag = fmt.Sprintf(" -d %d", self.LogLevel)
	}

	serviceCmd := fmt.Sprintf(
		"nohup sh -c '%s run%s -i %s%s; echo \"PROCESS EXITED rc=$?\" >&2' > %s 2>&1 &",
		binaryPath, debugFlag, configPath, listeners, logsPath)

	logrus.Infof("starting ziti-prox-c [%s]: %s", c.Id, serviceCmd)
	value, err := c.GetHost().ExecLogged(serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *ZitiProxCType) Stop(run model.Run, c *model.Component) error {
	if err := c.GetHost().KillProcesses("-TERM", self.getProcessFilter(c)); err != nil {
		return err
	}

	time.Sleep(2 * time.Second)

	if running, err := self.IsRunning(run, c); err != nil {
		return err
	} else if running {
		logrus.Infof("ziti-prox-c [%s] still running after SIGTERM, sending SIGKILL", c.Id)
		return c.GetHost().KillProcesses("-KILL", self.getProcessFilter(c))
	}

	return nil
}
