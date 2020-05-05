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

package ip

import (
	"fmt"
	"net"
)

// Parse a layer 3 packet (ipv4 or ipv6), and return
// - src, dst IP addresses,
// - layer 4 protocol
// - payload
// - error
func Decode(packet []byte) (net.IP, net.IP, int, []byte, error) {
	var l4Proto int
	var payload []byte
	var src, dst net.IP

	ipVersion := packet[0] >> 4
	switch ipVersion {
	case 4:
		h := IPv4(packet)
		src = h.SourceAddress()
		dst = h.DestinationAddress()
		l4Proto = int(h.Protocol())
		payload = h.Payload()
	default:
		return nil, nil, 0, nil, fmt.Errorf("unsupported IP version %d", ipVersion)
	}

	return src, dst, l4Proto, payload, nil
}
