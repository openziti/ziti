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
	"bytes"
	"github.com/michaelquigley/pfxlog"
	"github.com/pkg/errors"
	"net"
)

func IsLocallyAssigned(addr, lower, upper net.IP) bool {
	return bytes.Compare(addr.To16(), lower.To16()) >= 0 && bytes.Compare(addr.To16(), upper.To16()) <= 0
}

// return the next available IP address in the range of provided IPs
func NextIP(lower, upper net.IP) (net.IP, error) {
	usedAddrs, err := AllInterfaceAddrs()
	if err != nil {
		return nil, err
	}

	// need to make a copy of lower net.IP, since they're just byte arrays. Otherwise
	// we're continually changing the lower ip globally
	ip := net.IP(make([]byte, len(lower)))
	copy(ip, lower)

	for ; !ip.Equal(upper); IncIP(ip) {
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

// Return the length of a full prefix (no subnetting) for the given IP address.
// Returns 32 for ipv4 addresses, and 128 for ipv6 addresses.
func AddrBits(ip net.IP) int {
	if ip == nil {
		return 0
	} else if ip.To4() != nil {
		return net.IPv4len * 8
	} else if ip.To16() != nil {
		return net.IPv6len * 8
	}

	pfxlog.Logger().Infof("invalid IP address %s", ip.String())
	return 0
}

func Ip2IPnet(ip net.IP) *net.IPNet {
	prefixLen := AddrBits(ip)
	ipNet := &net.IPNet{IP: ip, Mask: net.CIDRMask(prefixLen, prefixLen)}
	return ipNet
}

func GetDialIP(addr string) (net.IP, *net.IPNet, error) {
	// hostname is an ip address, return it
	if parsedIP := net.ParseIP(addr); parsedIP != nil {
		ipNet := Ip2IPnet(parsedIP)
		return parsedIP, ipNet, nil
	}

	if parsedIP, cidr, err := net.ParseCIDR(addr); err == nil {
		return parsedIP, cidr, nil
	}

	return nil, nil, errors.Errorf("could not parse '%s' as IP or CIDR", addr)
}
