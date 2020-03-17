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
	"fmt"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	Root.AddCommand(clientCmd)
}

var clientCmd = &cobra.Command{
	Use: "client",
	Short: "start transwarp client",
	Run: client,
}

func client(_ *cobra.Command, _ []string) {
	serverAddress, err := transport.ParseAddress(serverAddressString)
	if err != nil {
		logrus.Fatalf("error resolving address (%v)", err)
	}

	conn, err := serverAddress.Dial("transwarp", nil)
	if err != nil {
		logrus.Fatalf("error dialing (%v)", err)
	}
	defer func() { _ = conn.Close() }()

	i := 0
	for {
		msg := fmt.Sprintf("hello #%d", i)
		logrus.Infof("sending [%s] to [%s]", msg, serverAddress)
		_, err := conn.Writer().Write([]byte(msg))
		if err != nil {
			logrus.Fatalf("error writing (%v)", err)
		}
		i++
		time.Sleep(200 * time.Millisecond)
	}
}