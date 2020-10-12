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

package intercept

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/tunnel/dns"
	"github.com/openziti/edge/tunnel/utils"
	"net"
)

var dnsIpLow, dnsIpHigh net.IP

func SetDnsInterceptIpRange(cidr string) error {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid cidr %s: %v", cidr, err)
	}

	var ips []net.IP
	for ip = ip.Mask(ipnet.Mask); ipnet.Contains(ip); utils.IncIP(ip) {
		a := make(net.IP, len(ip))
		copy(a, ip)
		ips = append(ips, a)
	}

	// remove network address and broadcast address
	dnsIpLow = ips[1]
	dnsIpHigh = ips[len(ips)-2]

	if len(dnsIpLow) != len(dnsIpHigh) {
		return fmt.Errorf("lower dns IP length %d differs from upper dns IP length %d", len(dnsIpLow), len(dnsIpHigh))
	}

	pfxlog.Logger().Infof("dns intercept IP range: %s - %s", dnsIpLow.String(), dnsIpHigh.String())
	return nil
}

func getInterceptIP(hostname string, resolver dns.Resolver) (net.IP, error) {
	log := pfxlog.Logger()
	// hostname is an ip address, return it
	parsedIP := net.ParseIP(hostname)
	if parsedIP != nil {
		return parsedIP, nil
	}

	// - apparently not an IP address, assume hostname and attempt lookup:
	// - use first result if any
	// - pin first IP to hostname in resolver
	addrs, err := net.LookupIP(hostname)
	if err == nil {
		if len(addrs) > 0 {
			_ = resolver.AddHostname(hostname, addrs[0].To4())
			return addrs[0], nil
		}
	} else {
		log.Debugf("net.LookupIp(%s) failed: %s", hostname, err)
	}

	ip, _ := utils.NextIP(dnsIpLow, dnsIpHigh)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address or unresolvable hostname: %s", hostname)
	}
	_ = resolver.AddHostname(hostname, ip)
	return ip, nil
}
