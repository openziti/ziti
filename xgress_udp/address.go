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

package xgress_udp

import (
	"fmt"
	"strings"
)

var supportedNetworks = map[string]string{
	"udp":      "",
	"udp4":     "",
	"udp6":     "",
	"unixgram": "",
}

func Parse(s string) (*PacketAddress, error) {
	tokens := strings.Split(s, ":")
	if len(tokens) < 2 {
		return nil, fmt.Errorf("invalid format")
	}

	network := tokens[0]

	if _, ok := supportedNetworks[network]; !ok {
		return nil, fmt.Errorf("unsupported network '%s'", network)
	}

	address := strings.Join(tokens[1:], ":")

	return &PacketAddress{network, address}, nil
}

func (pa *PacketAddress) String() string {
	return fmt.Sprintf("PacketAddress{network=[%v], addr=[%v]}", pa.network, pa.address)
}

func (pa *PacketAddress) Network() string {
	return pa.network
}

func (pa *PacketAddress) Address() string {
	return pa.address
}

type PacketAddress struct {
	network string
	address string
}
