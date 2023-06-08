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

package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"

	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/ziti/cmd"
	"github.com/sirupsen/logrus"
)

func Exec(m *model.Model, args ...string) (string, error) {
	if !m.IsBound() {
		return "", errors.New("model not bound")
	}

	var cliOut bytes.Buffer
	var cliErr bytes.Buffer

	ziticli := cmd.NewRootCommand(os.Stdin, &cliOut, &cliErr)
	ziticli.SetArgs(args)
	logrus.Infof("executing: %s", strings.Join(args, " "))
	if err := ziticli.Execute(); err != nil {
		logrus.Errorf("err executing command, err:[%e]", err)
	}

	return cliOut.String(), nil
}
