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
	"github.com/openziti/edge/tunnel/entities"
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

func cleanUpFunc(hostname string, resolver dns.Resolver) func() {
	f := func() {
		if err := resolver.RemoveHostname(hostname); err != nil {
			pfxlog.Logger().WithError(err).Errorf("failed to remove host mapping from resolver: %v ", hostname)
		}
	}
	return f
}

func getInterceptIP(svc *entities.Service, hostname string, resolver dns.Resolver, addrCB func(net.IP, *net.IPNet)) error {
	logger := pfxlog.Logger()

	if hostname[0] == '*' {
		err := resolver.AddDomain(hostname, func(host string) (net.IP, error) {
			var ip net.IP
			var err error
			ip, err = utils.NextIP(dnsIpLow, dnsIpHigh)

			if err == nil {
				addrCB(ip, utils.Ip2IPnet(ip))
				svc.AddCleanupAction(cleanUpFunc(host, resolver))
			}
			return ip, err
		})
		return err
	}

	ip, ipNet, err := utils.GetDialIP(hostname)
	if err == nil {
		addrCB(ip, ipNet)
		return err
	}

	ip, _ = utils.NextIP(dnsIpLow, dnsIpHigh)
	if ip == nil {
		return fmt.Errorf("invalid IP address or unresolvable hostname: %s", hostname)
	}
	if err = resolver.AddHostname(hostname, ip); err != nil {
		logger.WithError(err).Errorf("failed to add host/ip mapping to resolver: %v -> %v", hostname, ip)
	}

	svc.AddCleanupAction(cleanUpFunc(hostname, resolver))

	ipNet = utils.Ip2IPnet(ip)
	addrCB(ip, ipNet)
	return nil
}
