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
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
	"strings"
)

func getZitiProcessFilter(c *model.Component, zitiType string) func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "ziti") &&
			strings.Contains(s, zitiType) &&
			strings.Contains(s, fmt.Sprintf("--cli-agent-alias %s", c.Id)) &&
			!strings.Contains(s, "sudo ")
	}
}

func startZitiComponent(c *model.Component, zitiType string, version string, configName string) error {
	binaryName := "ziti"
	if version != "" {
		binaryName += "-" + version
	}

	factory := lib.NewSshConfigFactory(c.GetHost())

	binaryPath := fmt.Sprintf("/home/%s/fablab/bin/%s", factory.User(), binaryName)
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s", factory.User(), configName)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", factory.User(), c.Id)

	useSudo := ""
	if zitiType == "tunnel" || c.HasTag("tunneler") {
		useSudo = "sudo"
	}

	serviceCmd := fmt.Sprintf("nohup %s %s %s run --log-formatter pfxlog %s --cli-agent-alias %s > %s 2>&1 &",
		useSudo, binaryPath, zitiType, configPath, c.Id, logsPath)

	value, err := lib.RemoteExec(factory, serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func getPrefixVersion(version string) string {
	if version == "" || strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}
