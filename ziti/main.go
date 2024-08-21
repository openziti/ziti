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

package main

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/dtls"
	"github.com/openziti/transport/v2/tcp"
	"github.com/openziti/transport/v2/tls"
	"github.com/openziti/transport/v2/transwarp"
	"github.com/openziti/transport/v2/transwarptls"
	"github.com/openziti/transport/v2/udp"
	"github.com/openziti/transport/v2/ws"
	"github.com/openziti/transport/v2/wss"
	"github.com/openziti/ziti/common/build"
	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/ziti/cmd"
	"github.com/sirupsen/logrus"
)

func init() {
	options := pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").NoColor()
	pfxlog.GlobalInit(logrus.InfoLevel, options)

	transport.AddAddressParser(tls.AddressParser{})
	transport.AddAddressParser(dtls.AddressParser{})
	transport.AddAddressParser(tcp.AddressParser{})
	transport.AddAddressParser(transwarp.AddressParser{})
	transport.AddAddressParser(transwarptls.AddressParser{})
	transport.AddAddressParser(ws.AddressParser{})
	transport.AddAddressParser(wss.AddressParser{})
	transport.AddAddressParser(udp.AddressParser{})

	build.InitBuildInfo(version.GetCmdBuildInfo())
}

func main() {
	cmd.Execute()
}
