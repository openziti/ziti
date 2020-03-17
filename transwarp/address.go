/*
	(c) Copyright NetFoundry, Inc.

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

package transwarp

import (
	"fmt"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport"
	"io"
	"strconv"
	"strings"
)

func (self address) Dial(name string, _ *identity.TokenId) (transport.Connection, error) {
	return Dial(self.bindableAddress(), name)
}

func (self address) Listen(name string, _ *identity.TokenId, incoming chan transport.Connection) (io.Closer, error) {
	return Listen(self.bindableAddress(), name, incoming)
}

func (self address) MustListen(name string, _ *identity.TokenId, incoming chan transport.Connection) io.Closer {
	closer, err := self.Listen(name, nil, incoming)
	if err != nil {
		panic(fmt.Errorf("cannot listen (%w)", err))
	}
	return closer
}

func (self address) String() string {
	return fmt.Sprintf("transwarp:%s", self.bindableAddress())
}

func (self address) bindableAddress() string {
	return fmt.Sprintf("%s:%d", self.hostname, self.port)
}

type address struct {
	hostname string
	port     uint16
}

type AddressParser struct{}

func (_ AddressParser) Parse(s string) (transport.Address, error) {
	tokens := strings.Split(s, ":")
	if len(tokens) != 3 {
		return nil, fmt.Errorf("invalid format")
	}

	if tokens[0] == "transwarp" {
		port, err := strconv.ParseUint(tokens[2], 10, 16)
		if err != nil {
			return nil, err
		}
		return &address{hostname: tokens[1], port: uint16(port)}, nil
	}

	return nil, fmt.Errorf("invalid format")
}
