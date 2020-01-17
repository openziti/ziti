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

package xgress_udp

import (
	"errors"
	"fmt"
	"strings"
)

var supportedNetworks = map[string]string{
	"udp":      "",
	"udp4":     "",
	"udp6":     "",
	"unixgram": "",
}

type packetAddress struct {
	network string
	address string
}

func (pa *packetAddress) String() string {
	return fmt.Sprintf("packetAddress{network=%v, addr=%v}", pa.network, pa.address)
}

func (pa *packetAddress) Network() string {
	return pa.network
}

func (pa *packetAddress) Address() string {
	return pa.address
}

func parseAddress(s string) (*packetAddress, error) {
	tokens := strings.Split(s, ":")
	if len(tokens) < 2 {
		return nil, errors.New("invalid format")
	}

	network := tokens[0]

	if _, ok := supportedNetworks[network]; !ok {
		return nil, fmt.Errorf("unsupported network '%s'", network)
	}

	address := strings.Join(tokens[1:], ":")

	return &packetAddress{network, address}, nil
}
