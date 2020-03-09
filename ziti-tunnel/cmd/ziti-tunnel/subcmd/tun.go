// +build linux

/*
	Copyright 2019 NetFoundry, Inc.

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

package subcmd

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/tunnel/intercept/tun"
	"github.com/spf13/cobra"
)

const (
	tunMtuFlag    = "mtu"
	tunMtuDefault = 65535
)

func init() {
	tunCmd.PersistentFlags().Uint(tunMtuFlag, tunMtuDefault, "Set MTU of ephemeral tun interface")
	root.AddCommand(tunCmd)
}

var tunCmd = &cobra.Command{
	Use:     "tun <config>",
	Short:   "Intercept packets with tun interface",
	Args:    cobra.NoArgs,
	RunE:    runTun,
	PostRun: rootPostRun,
}

func runTun(cmd *cobra.Command, args []string) error {
	log := pfxlog.Logger()
	mtu, err := cmd.Flags().GetUint(tunMtuFlag)
	if err != nil {
		log.WithError(err).Warn("error getting MTU")
	}

	interceptor, err = tun.New("", mtu)
	if err != nil {
		return fmt.Errorf("failed to initialize tun interceptor: %v", err)
	}
	return nil
}
