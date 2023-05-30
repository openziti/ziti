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

package zitilib_runlevel_1_configuration

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/ziti/cmd"
	"github.com/sirupsen/logrus"
)

func generateCa(trustDomain string) error {
	if caExists, err := hasExistingCA("root"); caExists || err != nil {
		return err
	}

	var pkiOut bytes.Buffer
	var pkiErr bytes.Buffer
	ziticli := cmd.NewRootCommand(nil, &pkiOut, &pkiErr)
	args := []string{"pki", "create", "ca", "--pki-root", model.PkiBuild(), "--ca-name", "root", "--ca-file", "root", "--trust-domain", trustDomain, "--"}
	ziticli.SetArgs(args)
	logrus.Infof("%v", args)
	if err := ziticli.Execute(); err != nil {
		logrus.Errorf("stdOut [%s], stdErr [%s]", strings.Trim(pkiOut.String(), " \t\r\n"), strings.Trim(pkiErr.String(), " \t\r\n"))
		return fmt.Errorf("error generating key (%s)", err)
	}

	return nil
}

func generateSigningCert(name string) error {
	logrus.Infof("generating signing certificate [%s]", name)

	var pkiOut bytes.Buffer
	var pkiErr bytes.Buffer

	ziticli := cmd.NewRootCommand(nil, &pkiOut, &pkiErr)

	args := []string{"pki", "create", "intermediate", "--pki-root", model.PkiBuild(), "--ca-name", "root",
		"--intermediate-file", name, "--intermediate-name", name + " signing cert"}
	ziticli.SetArgs(args)
	logrus.Infof("%v", args)
	if err := ziticli.Execute(); err != nil {
		logrus.Errorf("stdOut [%s], stdErr [%s]", strings.Trim(pkiOut.String(), " \t\r\n"), strings.Trim(pkiErr.String(), " \t\r\n"))
		return fmt.Errorf("error generating key (%s)", err)
	}
	return nil
}

func generateCert(name, ip, spiffeId string) error {
	logrus.Infof("generating server certificate [%s:%s]", name, ip)

	var pkiOut bytes.Buffer
	var pkiErr bytes.Buffer
	ziticli := cmd.NewRootCommand(nil, &pkiOut, &pkiErr)

	args := []string{"pki", "create", "server",
		"--server-name", name,
		"--pki-root", model.PkiBuild(),
		"--ca-name", name,
		"--server-file", fmt.Sprintf("%s-server", name),
		"--ip", ip}

	if spiffeId != "" {
		args = append(args, "--spiffe-id", spiffeId)
	}

	logrus.Infof("%v", args)
	ziticli.SetArgs(args)
	if err := ziticli.Execute(); err != nil {
		logrus.Errorf("stdOut [%s], stdErr [%s]", strings.Trim(pkiOut.String(), " \t\r\n"), strings.Trim(pkiErr.String(), " \t\r\n"))
		return fmt.Errorf("error generating server certificate (%s)", err)
	}

	return nil
}
