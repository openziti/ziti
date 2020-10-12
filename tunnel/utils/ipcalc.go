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

package utils

import (
	"net"
)

// return the next available IP address in the range of provided IPs
func NextIP(lower, upper net.IP) (net.IP, error) {
	usedAddrs, err := AllInterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for ip := lower; !ip.Equal(upper); IncIP(ip) {
		inUse := false
		for _, usedAddr := range usedAddrs {
			usedIP, _, _ := net.ParseCIDR(usedAddr.String())
			if ip.Equal(usedIP) {
				inUse = true
				break
			}
		}
		if !inUse {
			return ip, nil
		}
	}

	return nil, nil
}

func IncIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] > 0 {
			break
		}
	}
}
