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

package intercept

import (
	"fmt"
	"github.com/gaissmai/extnetip"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/tunnel/dns"
	"github.com/openziti/ziti/tunnel/entities"
	"github.com/openziti/ziti/tunnel/utils"
	"net"
	"net/netip"
	"sync"
)

var dnsPrefix netip.Prefix
var dnsCurrentIp netip.Addr
var dnsCurrentIpMtx sync.Mutex

func SetDnsInterceptIpRange(cidr string) error {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("invalid cidr %s: %v", cidr, err)
	}

	dnsPrefix = prefix
	// get last ip in range for logging
	_, dnsIpHigh := extnetip.Range(dnsPrefix)

	dnsCurrentIpMtx.Lock()
	dnsCurrentIp = dnsPrefix.Addr()
	dnsCurrentIpMtx.Unlock()
	pfxlog.Logger().Infof("dns intercept IP range: %v - %v", dnsCurrentIp, dnsIpHigh)
	return nil
}

func GetDnsInterceptIpRange() *net.IPNet {
	return &net.IPNet{
		IP:   dnsPrefix.Addr().AsSlice(),
		Mask: net.CIDRMask(dnsPrefix.Bits(), dnsPrefix.Addr().BitLen()),
	}
}

func cleanUpFunc(hostname string, resolver dns.Resolver) func() {
	f := func() {
		if err := resolver.RemoveHostname(hostname); err != nil {
			pfxlog.Logger().WithError(err).Errorf("failed to remove host mapping from resolver: %v ", hostname)
		}
	}
	return f
}

func incDnsIp() (err error) {
	dnsCurrentIpMtx.Lock()
	defer dnsCurrentIpMtx.Unlock()
	ip := dnsCurrentIp.Next()
	if ip.IsValid() && dnsPrefix.Contains(ip) {
		dnsCurrentIp = ip
	} else {
		err = fmt.Errorf("cannot allocate ip address: ip range exhausted")
	}
	return
}

func getDnsIp(host string, addrCB func(*net.IPNet, bool), svc *entities.Service, resolver dns.Resolver) (net.IP, error) {
	err := incDnsIp()
	if err == nil {
		addr := &net.IPNet{
			IP:   dnsCurrentIp.AsSlice(),
			Mask: net.CIDRMask(dnsCurrentIp.BitLen(), dnsCurrentIp.BitLen()),
		}
		addrCB(addr, false)
		svc.AddCleanupAction(cleanUpFunc(host, resolver))
	}
	return dnsCurrentIp.AsSlice(), err
}

func getInterceptIP(svc *entities.Service, hostname string, resolver dns.Resolver, addrCB func(*net.IPNet, bool)) error {
	logger := pfxlog.Logger()

	// handle wildcard domain - IPs will be allocated when matching hostnames are queried
	if hostname[0] == '*' {
		err := resolver.AddDomain(hostname, func(host string) (net.IP, error) {
			return getDnsIp(hostname, addrCB, svc, resolver)
		})
		return err
	}

	// handle IP or CIDR
	ipNet, err := utils.GetCidr(hostname)
	if err == nil {
		addrCB(ipNet, true)
		return err
	}

	// handle hostnames
	ip, err := getDnsIp(hostname, addrCB, svc, resolver)
	if err != nil {
		return fmt.Errorf("invalid IP address or unresolvable hostname: %s", hostname)
	}
	if err = resolver.AddHostname(hostname, ip); err != nil {
		logger.WithError(err).Errorf("failed to add host/ip mapping to resolver: %v -> %v", hostname, ip)
	}

	return nil
}
