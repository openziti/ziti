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
	"github.com/netfoundry/ziti-foundation/transport"
	"net"
)

func Dial(destination, name string) (transport.Connection, error) {
	destinationAddress, err := net.ResolveUDPAddr("udp", destination)
	if err != nil {
		return nil, fmt.Errorf("error resolving destination address (%w)", err)
	}

	localAddress, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return nil, fmt.Errorf("error resolving local address (%w)", err)
	}

	conn, err := net.DialUDP("udp", localAddress, destinationAddress)
	if err != nil {
		return nil, fmt.Errorf("error dialing (%w)", err)
	}

	return newConnection(&transport.ConnectionDetail{
		Address: "transwarp" + destination,
		InBound: false,
		Name:    name,
	}, conn), nil
}
