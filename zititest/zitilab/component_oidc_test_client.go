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
	"encoding/json"
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
// In proxy mode, a single instance per host rotates through the host's local
// ziti-prox-c instances (discovered from sibling components at Start), dialing
// each by its explicit service-to-port mapping so emitted events identify the
// target prox by component ID.
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

	// ResultsService is the Ziti service name for reporting traffic results.
	ResultsService string
}

// destination and destinationsConfig mirror the oidc-test-client JSON schema
// (see zititest/oidc-test-client/main.go). We emit this file to the remote host
// at Start time so the driver knows exactly which prox instances to exercise
// and under what ClientId to report them.
type destination struct {
	ClientId string         `json:"client_id"`
	Ports    map[string]int `json:"ports,omitempty"`
}

type destinationsConfig struct {
	Destinations []destination `json:"destinations"`
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
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s.json", user, c.Id)

	mode := self.modeString()
	args := fmt.Sprintf("--mode %s --services %s", mode, self.Services)

	if self.Mode == OidcTestClientSdkDirect {
		args += fmt.Sprintf(" --identity %s --client-id %s", configPath, c.Id)
	} else {
		destFile := fmt.Sprintf("/home/%s/fablab/cfg/%s-destinations.json", user, c.Id)
		if err := self.writeDestinationsFile(c, destFile); err != nil {
			return fmt.Errorf("write destinations for %s: %w", c.Id, err)
		}
		args += fmt.Sprintf(" --destinations-file %s --results-identity %s", destFile, configPath)
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

	// Keep stderr on disk so panics (which the Go runtime writes to stderr,
	// not through logrus) are visible after a silent process exit.
	stderrPath := fmt.Sprintf("/home/%s/logs/%s.err", user, c.Id)
	serviceCmd := fmt.Sprintf("nohup %s %s > /dev/null 2>>%s &", binaryPath, args, stderrPath)

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

// writeDestinationsFile enumerates the host's ziti-prox-c siblings and writes
// a JSON mapping (remote path) of each prox's service-to-port map keyed by the
// prox's component ID. The traffic driver rotates through this list so events
// can identify the target prox precisely.
func (self *OidcTestClientType) writeDestinationsFile(c *model.Component, remotePath string) error {
	var dests []destination
	for _, sibling := range c.GetHost().Components {
		prox, ok := sibling.Type.(*ZitiProxCType)
		if !ok {
			continue
		}
		idx, err := parseInstanceIndex(sibling.Id)
		if err != nil {
			return fmt.Errorf("parse instance index for prox %s: %w", sibling.Id, err)
		}
		ports := make(map[string]int, len(prox.Services))
		for svcIdx, svc := range prox.Services {
			ports[svc] = prox.BasePort + idx*len(prox.Services) + svcIdx
		}
		dests = append(dests, destination{ClientId: sibling.Id, Ports: ports})
	}
	if len(dests) == 0 {
		return fmt.Errorf("no ziti-prox-c siblings found on host %s", c.GetHost().GetId())
	}

	payload, err := json.MarshalIndent(destinationsConfig{Destinations: dests}, "", "  ")
	if err != nil {
		return err
	}

	// Write via a heredoc so the remote filesystem gets an exact copy without
	// needing a separate upload step.
	cmd := fmt.Sprintf("mkdir -p $(dirname %s) && cat > %s <<'EOF_DESTS'\n%s\nEOF_DESTS\n", remotePath, remotePath, string(payload))
	if _, err := c.GetHost().ExecLogged(cmd); err != nil {
		return fmt.Errorf("uploading destinations file: %w", err)
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
