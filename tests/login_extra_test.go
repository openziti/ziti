//go:build cli_tests

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
package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/run"
	"github.com/openziti/ziti/ziti/util"
	"github.com/sirupsen/logrus"
)

func (o *overlay) Login() (*edge.LoginOptions, error) {
	initialLogin := &edge.LoginOptions{
		Options:       commonOpts,
		Username:      username,
		Password:      password,
		ControllerUrl: o.controllerHostPort(),
		Yes:           true,
		NetworkId:     networkIdFile,
	}
	il, ile := initialLogin, initialLogin.Run()
	if ile == nil {
		util.ReloadConfig() //every login really needs to call reload to flush/overwrite the cached client
	}
	return il, ile
}

func (o *overlay) LaunchOverlayOnlyController(t *testing.T, ctx context.Context) {
	fmt.Println("Starting controller...")
	go func() {
		runCtrl := run.NewRunControllerCmd()
		runCtrl.SetArgs([]string{
			o.ConfigFile,
		})
		runCtrl.SetContext(ctx)
		runCtrlErr := runCtrl.Execute()
		if runCtrlErr != nil {
			logrus.Fatal(runCtrlErr)
		}
	}()
	fmt.Println("Controller running...")
}

type externalOverlay interface {
	Start() error
	Stop() error
	WaitForReady() error
	WaitForStop() error
}
