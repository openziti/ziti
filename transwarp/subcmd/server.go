/*
	Copyright 2020 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	Root.AddCommand(serverCmd)
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "start transwarp server",
	Run:   server,
}

func server(_ *cobra.Command, _ []string) {
	serverAddress, err := transport.ParseAddress(serverAddressString)
	if err != nil {
		logrus.Fatalf("error parsing address (%w)", err)
	}

	incoming := make(chan transport.Connection, 1)
	_, err = serverAddress.Listen("transwarp", nil, incoming)
	if err != nil {
		logrus.Fatalf("error listening (%w)", err)
	}
	connection := <-incoming

	buffer := make([]byte, 10240)
	for {
		n, peer, err := connection.ReadPeer(buffer)
		if err != nil {
			logrus.Errorf("error reading (%v)", err)
		} else {
			logrus.Infof("received [%s] from [%s]", string(buffer[:n]), peer)
		}
	}
}
