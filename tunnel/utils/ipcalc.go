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

package utils

import (
	"fmt"
	"net"
	"net/netip"
)

func GetCidr(ipOrCidr string) (*net.IPNet, error) {
	ip, err := netip.ParseAddr(ipOrCidr)
	if err == nil {
		return &net.IPNet{
			IP:   ip.AsSlice(),
			Mask: net.CIDRMask(ip.BitLen(), ip.BitLen()),
		}, nil
	}

	pfx, err := netip.ParsePrefix(ipOrCidr)
	if err == nil {
		return &net.IPNet{
			IP:   pfx.Addr().AsSlice(),
			Mask: net.CIDRMask(pfx.Bits(), pfx.Addr().BitLen()),
		}, nil
	}

	return nil, fmt.Errorf("failed to parse '%v' as IP or CIDR", ipOrCidr)
}
