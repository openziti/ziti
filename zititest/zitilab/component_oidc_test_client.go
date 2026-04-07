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
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
)

var _ model.ComponentType = (*OidcTestClientType)(nil)

// OidcTestClientMode selects how the client connects to the overlay.
type OidcTestClientMode int

const (
	// OidcTestClientSdkDirect uses the Go SDK to dial services directly.
	OidcTestClientSdkDirect OidcTestClientMode = 0

	// OidcTestClientProxy connects through local ziti-prox-c proxy instances,
	// rotating through a port range to exercise every proxy on the host.
	OidcTestClientProxy OidcTestClientMode = 1
)

// OidcTestClientType wraps the oidc-test-client binary as a fablab component.
// In sdk-direct mode, each instance authenticates via OIDC and dials services.
// In proxy mode, a single instance per host rotates through all prox-c proxy
// ports, verifying every proxy can carry traffic.
type OidcTestClientType struct {
	// Mode selects sdk-direct or proxy mode.
	Mode OidcTestClientMode

	// LocalPath overrides the binary path (for local builds).
	LocalPath string

	// Services is the comma-separated list of Ziti service names to test.
	Services string

	// DialInterval is the time between short-lived dials (e.g., "30s").
	DialInterval string

	// HeartbeatInterval is the time between heartbeats on the long-lived connection (e.g., "5s").
	HeartbeatInterval string

	// ProxyBasePort is the starting port for proxy instance rotation (proxy mode only).
	ProxyBasePort int

	// ProxyInstanceCount is the number of proxy instances to rotate through (proxy mode only).
	ProxyInstanceCount int

	// ResultsService is the Ziti service name for reporting traffic results.
	ResultsService string
}

func (self *OidcTestClientType) Label() string {
	return "oidc-test-client"
}

func (self *OidcTestClientType) GetVersion() string {
	return ""
}

func (self *OidcTestClientType) SetVersion(string) {}

func (self *OidcTestClientType) InitType(*model.Component) {}

func (self *OidcTestClientType) Dump() any {
	return map[string]string{
		"type_id":    "oidc-test-client",
		"mode":       self.modeString(),
		"services":   self.Services,
		"local_path": self.LocalPath,
	}
}

func (self *OidcTestClientType) StageFiles(r model.Run, c *model.Component) error {
	return stageziti.StageLocalOnce(r, "oidc-test-client", c, self.LocalPath)
}

func (self *OidcTestClientType) getProcessFilter(c *model.Component) func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "oidc-test-client") &&
			strings.Contains(s, fmt.Sprintf("%s.json", c.Id))
	}
}

func (self *OidcTestClientType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *OidcTestClientType) Start(run model.Run, c *model.Component) error {
	if running, err := self.IsRunning(run, c); err != nil {
		return err
	} else if running {
		logrus.Infof("oidc-test-client [%s] already running", c.Id)
		return nil
	}

	user := c.GetHost().GetSshUser()

	binaryPath := fmt.Sprintf("/home/%s/fablab/bin/oidc-test-client", user)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	mode := self.modeString()
	args := fmt.Sprintf("--mode %s --services %s", mode, self.Services)

	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s.json", user, c.Id)

	if self.Mode == OidcTestClientSdkDirect {
		args += fmt.Sprintf(" --identity %s", configPath)
	} else {
		args += fmt.Sprintf(" --proxy-base-port %d --proxy-instance-count %d",
			self.ProxyBasePort, self.ProxyInstanceCount)
		args += fmt.Sprintf(" --results-identity %s", configPath)
	}

	if self.ResultsService != "" {
		args += fmt.Sprintf(" --results-service %s", self.ResultsService)
	}

	if self.DialInterval != "" {
		args += fmt.Sprintf(" --dial-interval %s", self.DialInterval)
	}
	if self.HeartbeatInterval != "" {
		args += fmt.Sprintf(" --heartbeat-interval %s", self.HeartbeatInterval)
	}

	args += fmt.Sprintf(" --log-file %s", logsPath)

	serviceCmd := fmt.Sprintf("nohup %s %s > /dev/null 2>&1 &", binaryPath, args)

	logrus.Infof("starting oidc-test-client [%s]: %s", c.Id, serviceCmd)
	value, err := c.GetHost().ExecLogged(serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func (self *OidcTestClientType) Stop(_ model.Run, c *model.Component) error {
	return c.GetHost().KillProcesses("-TERM", self.getProcessFilter(c))
}

func (self *OidcTestClientType) modeString() string {
	if self.Mode == OidcTestClientProxy {
		return "proxy"
	}
	return "sdk-direct"
}
