/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/foundation/transport"
	"github.com/openziti/foundation/transport/quic"
	"github.com/openziti/foundation/transport/tcp"
	"github.com/openziti/foundation/transport/tls"
	"github.com/openziti/foundation/transport/transwarp"
	"github.com/openziti/foundation/transport/transwarptls"
	"github.com/openziti/foundation/transport/wss"
	"github.com/openziti/ziti/ziti-fabric-gw/subcmd"
	"github.com/sirupsen/logrus"
)

func init() {
	pfxlog.GlobalInit(logrus.InfoLevel, pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").NoColor())
	transport.AddAddressParser(quic.AddressParser{})
	transport.AddAddressParser(tls.AddressParser{})
	transport.AddAddressParser(tcp.AddressParser{})
	transport.AddAddressParser(transwarp.AddressParser{})
	transport.AddAddressParser(transwarptls.AddressParser{})
	transport.AddAddressParser(wss.AddressParser{})
}

func main() {
	subcmd.Execute()
}
