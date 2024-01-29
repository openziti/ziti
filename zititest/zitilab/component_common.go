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
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/sirupsen/logrus"
	"path/filepath"
	"strings"
)

func getZitiProcessFilter(c *model.Component, zitiType string) func(string) bool {
	return func(s string) bool {
		return strings.Contains(s, "ziti") &&
			strings.Contains(s, zitiType) &&
			strings.Contains(s, fmt.Sprintf("--cli-agent-alias %s ", c.Id)) &&
			!strings.Contains(s, "sudo ")
	}
}

func startZitiComponent(c *model.Component, zitiType string, version string, configName string) error {
	user := c.GetHost().GetSshUser()

	binaryPath := getZitiBinaryPath(c, version)
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s", user, configName)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	useSudo := ""
	if zitiType == "tunnel" || c.HasTag("tunneler") {
		useSudo = "sudo"
	}

	serviceCmd := fmt.Sprintf("nohup %s %s %s run --cli-agent-alias %s --log-formatter pfxlog %s > %s 2>&1 &",
		useSudo, binaryPath, zitiType, c.Id, configPath, logsPath)
	logrus.Info(serviceCmd)
	value, err := c.GetHost().ExecLogged(serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func canonicalizeZitiVersion(version *string) {
	if version != nil {
		if *version != "" && *version != "latest" && !strings.HasPrefix(*version, "v") {
			*version = "v" + *version
		}
	}
}

func getZitiBinaryPath(c *model.Component, version string) string {
	binaryName := "ziti"
	if version != "" {
		binaryName += "-" + version
	}

	user := c.GetHost().GetSshUser()

	return fmt.Sprintf("/home/%s/fablab/bin/%s", user, binaryName)
}

func reEnrollIdentity(run model.Run, c *model.Component, zitiBinaryPath string, configPath string) error {
	if err := zitilib_actions.EdgeExec(run.GetModel(), "delete", "authenticator", "where", fmt.Sprintf("identity=\"%v\"", c.Id)); err != nil {
		return err
	}

	if err := zitilib_actions.EdgeExec(run.GetModel(), "delete", "enrollment", "where", fmt.Sprintf("identity=\"%v\"", c.Id)); err != nil {
		return err
	}

	jwtFileName := filepath.Join(model.ConfigBuild(), c.Id+".jwt")

	args := []string{"create", "enrollment", "ott", "--jwt-output-file", jwtFileName, "--", c.Id}

	if err := zitilib_actions.EdgeExec(c.GetModel(), args...); err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	remoteJwt := configDir + c.Id + ".jwt"
	if err := c.GetHost().SendFile(jwtFileName, remoteJwt); err != nil {
		return err
	}

	tmpl := "set -o pipefail; mkdir -p %s; %s edge enroll %s -o %s 2>&1 | tee /home/ubuntu/logs/%s.identity.enroll.log "
	cmd := fmt.Sprintf(tmpl, configDir, zitiBinaryPath, remoteJwt, configPath, c.Id)

	return c.GetHost().ExecLogOnlyOnError(cmd)
}
