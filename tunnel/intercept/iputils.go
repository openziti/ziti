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
	"container/list"
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
var dnsRecycledIps *list.List

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
	dnsRecycledIps = list.New()
	dnsCurrentIpMtx.Unlock()
	pfxlog.Logger().Infof("dns intercept IP range: %v - %v", dnsCurrentIp, dnsIpHigh)
	return nil
}

func GetDnsInterceptIpRange() *net.IPNet {
	if !dnsPrefix.IsValid() {
		if err := SetDnsInterceptIpRange("100.64.0.1/10"); err != nil {
			pfxlog.Logger().WithError(err).Errorf("Failed to set DNS intercept range")
		}
	}
	return &net.IPNet{
		IP:   dnsPrefix.Addr().AsSlice(),
		Mask: net.CIDRMask(dnsPrefix.Bits(), dnsPrefix.Addr().BitLen()),
	}
}

// cleanUpFunc returns the per-service cleanup action registered when a hostname
// is intercepted. The wrapping RefCountingResolver only forwards
// RemoveHostname to the underlying resolver on the last release, so this
// cleanup recycles the CGNAT IP only when the returned IP is non-nil.
func cleanUpFunc(hostname string, resolver dns.Resolver) func() {
	return func() {
		ip := resolver.RemoveHostname(hostname)
		if ip != nil {
			dnsCurrentIpMtx.Lock()
			defer dnsCurrentIpMtx.Unlock()
			addr, _ := netip.AddrFromSlice(ip)
			dnsRecycledIps.PushBack(addr)
		}
	}
}

// hostMask returns a host-route mask (/32 or /128) for the given IP.
func hostMask(ip net.IP) net.IPMask {
	bits := len(ip) * 8
	return net.CIDRMask(bits, bits)
}

func getDnsIp(host string, addrCB func(*net.IPNet, bool), svc *entities.Service, resolver dns.Resolver) (net.IP, error) {
	addr, cleanup, err := allocateDnsIp(host, resolver)
	if err != nil {
		return nil, err
	}
	// addrCB and AddCleanupAction must run outside dnsCurrentIpMtx: addrCB can
	// touch interceptor state, and AddCleanupAction takes service.lock which
	// is acquired in the reverse order by RunCleanupActions -> cleanUpFunc.
	addrCB(addr, false) // no route is needed because the dns cidr was added to "lo" at startup
	svc.AddCleanupAction(cleanup)
	return addr.IP, nil
}

// allocateDnsIp returns the IP mapped to host, allocating one from the
// intercept range if the hostname is unknown, and registers the hostname -> IP
// mapping with the resolver. Lookup and registration both happen under
// dnsCurrentIpMtx so concurrent allocations of the same hostname (e.g. a
// wildcard-domain DNS query racing a service update) can't both miss the
// lookup and allocate distinct IPs. Every call takes one reference on the
// hostname (see dns.NewRefCountingResolver); the returned cleanup releases it.
func allocateDnsIp(host string, resolver dns.Resolver) (*net.IPNet, func(), error) {
	dnsCurrentIpMtx.Lock()
	defer dnsCurrentIpMtx.Unlock()

	// If the hostname already has an allocated IP, reuse it, taking an
	// additional reference. Even though no new IP is allocated, the caller
	// still must invoke addrCB so the interceptor installs its per-service
	// iptables rule; otherwise the tproxy listener exists with no kernel rule
	// pointing at it and the service is silently unreachable.
	if foundIP, found := resolver.LookupIP(host + "."); found {
		if err := resolver.AddHostname(host, foundIP); err != nil {
			pfxlog.Logger().WithError(err).Errorf("failed to add host/ip mapping to resolver: %v -> %v", host, foundIP)
		}
		return &net.IPNet{IP: foundIP, Mask: hostMask(foundIP)}, cleanUpFunc(host, resolver), nil
	}

	var ip netip.Addr
	// look for returned IPs first
	if dnsRecycledIps.Len() > 0 {
		e := dnsRecycledIps.Front()
		ip = e.Value.(netip.Addr)
		dnsRecycledIps.Remove(e)
		pfxlog.Logger().Debugf("using recycled ip %v for hostname %s", ip, host)
	} else {
		ip = dnsCurrentIp.Next()
		if ip.IsValid() && dnsPrefix.Contains(ip) {
			dnsCurrentIp = ip
		} else {
			return nil, nil, fmt.Errorf("cannot allocate ip address: ip range exhausted")
		}
	}

	ipBytes := ip.AsSlice()
	if err := resolver.AddHostname(host, ipBytes); err != nil {
		pfxlog.Logger().WithError(err).Errorf("failed to add host/ip mapping to resolver: %v -> %v", host, ip)
	}
	return &net.IPNet{IP: ipBytes, Mask: hostMask(ipBytes)}, cleanUpFunc(host, resolver), nil
}

func getInterceptIP(svc *entities.Service, hostname string, resolver dns.Resolver, addrCB func(*net.IPNet, bool)) error {
	// handle wildcard domain - IPs will be allocated when matching hostnames are queried
	if hostname[0] == '*' {
		err := resolver.AddDomain(hostname, func(host string) (net.IP, error) {
			return getDnsIp(host, addrCB, svc, resolver)
		})
		if err == nil {
			svc.AddCleanupAction(func() { resolver.RemoveDomain(hostname) })
		}
		return err
	}

	// handle IP or CIDR
	ipNet, err := utils.GetCidr(hostname)
	if err == nil {
		addrCB(ipNet, true)
		return err
	}

	// handle hostnames. getDnsIp registers the hostname -> IP mapping with
	// the resolver as part of allocation.
	if _, err = getDnsIp(hostname, addrCB, svc, resolver); err != nil {
		return fmt.Errorf("invalid IP address or unresolvable hostname: %s", hostname)
	}

	return nil
}
