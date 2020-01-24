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

package main

import (
	"github.com/netfoundry/ziti-cmd/ziti-fabric-test/subcmd"
	_ "github.com/netfoundry/ziti-cmd/ziti-fabric-test/subcmd/client"
	_ "github.com/netfoundry/ziti-cmd/ziti-fabric-test/subcmd/loop2"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/netfoundry/ziti-foundation/transport/quic"
	"github.com/netfoundry/ziti-foundation/transport/tcp"
	"github.com/netfoundry/ziti-foundation/transport/tls"
	"github.com/netfoundry/ziti-foundation/util/info"
	"github.com/netfoundry/ziti-foundation/transport/udp"
	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
	"math/rand"
)

func init() {
	pfxlog.Global(logrus.InfoLevel)
	pfxlog.SetPrefix("bitbucket.org/netfoundry/")
	transport.AddAddressParser(quic.AddressParser{})
	transport.AddAddressParser(tls.AddressParser{})
	transport.AddAddressParser(tcp.AddressParser{})
	transport.AddAddressParser(udp.AddressParser{})
}

func main() {
	rand.Seed(info.NowInMilliseconds())
	subcmd.Execute()
}
