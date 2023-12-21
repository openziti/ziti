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

package pki

import (
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/ziti/cmd"
	"github.com/sirupsen/logrus"
)

func HasExistingCA(run model.Run, name string) (bool, error) {
	return run.DirExists(filepath.Join(model.BuildKitDir, model.BuildPkiDir, name))
}

func EnsureCaExists(run model.Run, trustDomain string, name string) error {
	if caExists, err := HasExistingCA(run, name); caExists || err != nil {
		return err
	}

	return GenerateCA(run, trustDomain, name)
}

func GenerateCA(run model.Run, trustDomain string, name string) error {
	var pkiOut bytes.Buffer
	var pkiErr bytes.Buffer
	ziticli := cmd.NewRootCommand(nil, &pkiOut, &pkiErr)
	args := []string{"pki", "create", "ca", "--pki-root", run.GetPkiDir(), "--ca-name", name, "--ca-file", name, "--trust-domain", trustDomain, "--"}
	ziticli.SetArgs(args)
	logrus.Infof("%v", args)
	if err := ziticli.Execute(); err != nil {
		logrus.Errorf("stdOut [%s], stdErr [%s]", strings.Trim(pkiOut.String(), " \t\r\n"), strings.Trim(pkiErr.String(), " \t\r\n"))
		return errors.Wrapf(err, "error generating ca '%s'", name)
	}

	return nil
}

func EnsureIntermediateCaExists(run model.Run, caName, name string) error {
	logrus.Infof("generating signing certificate [%s]", name)

	if caExists, err := HasExistingCA(run, name); caExists || err != nil {
		return err
	}

	var pkiOut bytes.Buffer
	var pkiErr bytes.Buffer

	ziticli := cmd.NewRootCommand(nil, &pkiOut, &pkiErr)

	args := []string{"pki", "create", "intermediate", "--pki-root", run.GetPkiDir(), "--ca-name", caName,
		"--intermediate-file", name, "--intermediate-name", name + " signing cert"}
	ziticli.SetArgs(args)
	logrus.Infof("%v", args)
	if err := ziticli.Execute(); err != nil {
		logrus.Errorf("stdOut [%s], stdErr [%s]", strings.Trim(pkiOut.String(), " \t\r\n"), strings.Trim(pkiErr.String(), " \t\r\n"))
		return fmt.Errorf("error generating key (%s)", err)
	}
	return nil
}

func loadInfoFile(file string) string {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		logrus.WithError(err).Infof("unable to read cert info file '%s'", file)
	}
	return string(data)
}

func EnsureServerCertExists(run model.Run, name string, ip string, dns []string, spiffeId string) error {
	logrus.Infof("generating server certificate [%s:%s]", name, ip)

	certFile := name + "-server"
	certFilePath := filepath.Join(model.BuildKitDir, model.BuildPkiDir, name, "certs", certFile)

	certExists, err := run.FileExists(certFilePath + ".cert")
	if err != nil {
		return err
	}

	certInfo := ip + "\n" + spiffeId + "\n"
	infoFile := filepath.Join(run.GetWorkingDir(), certFilePath+".info")

	if certExists {
		cachedInfo := loadInfoFile(infoFile)
		if certInfo == cachedInfo {
			logrus.Infof("server cert %v already up to date", name)
			return nil
		}
	}

	var pkiOut bytes.Buffer
	var pkiErr bytes.Buffer
	ziticli := cmd.NewRootCommand(nil, &pkiOut, &pkiErr)

	args := []string{"pki", "create", "server",
		"--server-name", name,
		"--pki-root", run.GetPkiDir(),
		"--ca-name", name,
		"--server-file", certFile,
		"--allow-overwrite",
		"--ip", ip}

	if len(dns) > 0 {
		args = append(args, "--dns", strings.Join(dns, ","))
	}

	if spiffeId != "" {
		args = append(args, "--spiffe-id", spiffeId)
	}

	logrus.Infof("%v", args)
	ziticli.SetArgs(args)
	if err := ziticli.Execute(); err != nil {
		logrus.Errorf("stdOut [%s], stdErr [%s]", strings.Trim(pkiOut.String(), " \t\r\n"), strings.Trim(pkiErr.String(), " \t\r\n"))
		return fmt.Errorf("error generating server certificate (%s)", err)
	}

	return os.WriteFile(infoFile, []byte(certInfo), 0600)
}

func CreateControllerCerts(run model.Run, component *model.Component, dns []string, name string) error {
	trustDomain := component.GetStringVariableOr("ca.trustDomain", "ziti.test")
	rootCaName := component.GetStringVariableOr("ca.rootName", "root")
	if err := EnsureCaExists(run, trustDomain, rootCaName); err != nil {
		return fmt.Errorf("error generating ca (%s)", err)
	}

	if err := EnsureIntermediateCaExists(run, rootCaName, name); err != nil {
		return errors.Wrapf(err, "error generating public identity for component [%s]", component.Id)
	}

	return EnsureServerCertExists(run, name, component.Host.PublicIp, dns, "controller/"+name)
}
