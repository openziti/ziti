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
	"net"
	"strings"
	"testing"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/tunnel/dns"
	"github.com/openziti/ziti/v2/tunnel/entities"
	"github.com/stretchr/testify/require"
)

// fakeResolver is a minimal in-memory dns.Resolver. Only the methods the
// tests exercise are implemented; the rest panic so an accidental dependency
// on more behavior fails loudly. In tests this is wrapped with
// dns.NewRefCountingResolver so the refcounting layer is exercised end to end.
type fakeResolver struct {
	names map[string]net.IP
}

func newFakeResolver() *fakeResolver {
	return &fakeResolver{names: map[string]net.IP{}}
}

func (r *fakeResolver) LookupIP(name string) (net.IP, bool) {
	ip, ok := r.names[strings.ToLower(name)]
	return ip, ok
}

func (r *fakeResolver) AddHostname(hostname string, ip net.IP) error {
	r.names[strings.ToLower(hostname)+"."] = ip
	return nil
}

func (r *fakeResolver) RemoveHostname(hostname string) net.IP {
	key := strings.ToLower(hostname) + "."
	ip, ok := r.names[key]
	if !ok {
		return nil
	}
	delete(r.names, key)
	return ip
}

func (r *fakeResolver) AddDomain(string, func(string) (net.IP, error)) error {
	panic("unused")
}
func (r *fakeResolver) Lookup(net.IP) (string, error) { panic("unused") }
func (r *fakeResolver) RemoveDomain(string)           { panic("unused") }
func (r *fakeResolver) Cleanup() error                { panic("unused") }

// resetDnsState resets the package-level allocation state so each test starts
// from a known baseline. The intercept package keeps allocator state in
// globals, and other tests in the package may run first.
func resetDnsState(t *testing.T) {
	t.Helper()
	require.NoError(t, SetDnsInterceptIpRange("100.64.0.1/10"))
}

// addInterceptHostname simulates getInterceptIP's handling of a direct
// hostname by calling getDnsIp, which allocates (or reuses) the IP and
// registers the hostname -> IP mapping with the resolver as part of
// allocation.
func addInterceptHostname(t *testing.T, host string, addrCB func(*net.IPNet, bool), svc *entities.Service, resolver dns.Resolver) net.IP {
	t.Helper()
	ip, err := getDnsIp(host, addrCB, svc, resolver)
	require.NoError(t, err)
	return ip
}

// Test_GetDnsIp_SharedHostnameInstallsRule asserts that when two services share
// an intercept hostname, getDnsIp invokes addrCB for both services so each one
// gets its iptables rule installed (the regression in #3867). Without the fix
// the second service silently has no rule programmed and is unreachable.
func Test_GetDnsIp_SharedHostnameInstallsRule(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := dns.NewRefCountingResolver(newFakeResolver())
	svcA := &entities.Service{}
	svcB := &entities.Service{}

	var callsA, callsB []*net.IPNet
	cbA := func(ipNet *net.IPNet, _ bool) { callsA = append(callsA, ipNet) }
	cbB := func(ipNet *net.IPNet, _ bool) { callsB = append(callsB, ipNet) }

	ipA := addInterceptHostname(t, "shared.example", cbA, svcA, resolver)
	req.Len(callsA, 1, "addrCB must fire for the first service")
	req.True(ipA.Equal(callsA[0].IP), "addrCB receives the allocated IP")

	ipB := addInterceptHostname(t, "shared.example", cbB, svcB, resolver)
	req.True(ipA.Equal(ipB), "second service must reuse the first service's IP")
	req.Len(callsB, 1, "addrCB must fire for the second service so its iptables rule is installed")
	req.True(ipB.Equal(callsB[0].IP), "addrCB for second service receives the shared IP")
}

// Test_GetDnsIp_RefCountedCleanup asserts that the hostname/IP allocation is
// reference counted so a service cleanup does not pull the rug out from under
// other services still using the same hostname.
func Test_GetDnsIp_RefCountedCleanup(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := dns.NewRefCountingResolver(newFakeResolver())
	svcA := &entities.Service{}
	svcB := &entities.Service{}
	noopCB := func(*net.IPNet, bool) {}

	addInterceptHostname(t, "shared.example", noopCB, svcA, resolver)
	addInterceptHostname(t, "shared.example", noopCB, svcB, resolver)

	// Cleaning up the second service must not remove the hostname or recycle
	// the IP -- the first service still depends on both.
	svcB.RunCleanupActions()
	_, found := resolver.LookupIP("shared.example.")
	req.True(found, "hostname must remain while another service still uses it")
	req.Equal(0, dnsRecycledIps.Len(), "IP must not be recycled while another service uses it")

	// Cleaning up the last service must remove the hostname and recycle the IP.
	svcA.RunCleanupActions()
	_, found = resolver.LookupIP("shared.example.")
	req.False(found, "hostname must be removed when the last service cleans up")
	req.Equal(1, dnsRecycledIps.Len(), "IP must be recycled when the last service cleans up")
}

// Test_GetDnsIp_SharedHostnameDifferentCase asserts that two services whose
// hostnames differ only in case share a single allocation, matching the
// resolver's case-insensitive view.
func Test_GetDnsIp_SharedHostnameDifferentCase(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := dns.NewRefCountingResolver(newFakeResolver())
	svcA := &entities.Service{}
	svcB := &entities.Service{}
	noopCB := func(*net.IPNet, bool) {}

	ipA := addInterceptHostname(t, "Shared.Example", noopCB, svcA, resolver)
	ipB := addInterceptHostname(t, "shared.example", noopCB, svcB, resolver)
	req.True(ipA.Equal(ipB), "case variants must resolve to the same IP")

	// First cleanup must not recycle: both services share the allocation.
	svcA.RunCleanupActions()
	req.Equal(0, dnsRecycledIps.Len(), "IP must not be recycled while a case variant still holds it")

	// Final cleanup recycles.
	svcB.RunCleanupActions()
	req.Equal(1, dnsRecycledIps.Len(), "IP must be recycled once the last case variant releases it")
}

// Test_GetDnsIp_CleanupOutOfOrder asserts that cleaning up the
// first-registered service does not strand other services still using the
// same hostname. Before the fix, the first service's cleanUpFunc was the
// only one registered for a shared hostname and would unconditionally remove
// the resolver entry and recycle the IP.
func Test_GetDnsIp_CleanupOutOfOrder(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := dns.NewRefCountingResolver(newFakeResolver())
	svcA := &entities.Service{}
	svcB := &entities.Service{}
	noopCB := func(*net.IPNet, bool) {}

	addInterceptHostname(t, "shared.example", noopCB, svcA, resolver)
	addInterceptHostname(t, "shared.example", noopCB, svcB, resolver)

	svcA.RunCleanupActions()
	_, found := resolver.LookupIP("shared.example.")
	req.True(found, "hostname must remain when an earlier-registered service cleans up while others still use it")
	req.Equal(0, dnsRecycledIps.Len(), "IP must not be recycled while another service uses it")
}

// Test_GetDnsIp_FromWildcardLambda asserts that getDnsIp works the same way
// when invoked from the closure registered via resolver.AddDomain for a
// wildcard intercept -- the lambda path getInterceptIP uses for hostnames
// starting with '*'. The closure must allocate an IP, register the hostname
// through the refcounting layer, install the per-service iptables rule via
// addrCB, and register a refcount-aware cleanup so reusing the same name from
// another service shares the allocation. This also covers the
// wildcard-then-literal overlap with the wildcard service removed first:
// before allocation took a reference, the wildcard cleanup stole the literal
// service's reference, removing the hostname while it was still intercepted.
func Test_GetDnsIp_FromWildcardLambda(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := dns.NewRefCountingResolver(newFakeResolver())
	svcA := &entities.Service{}
	svcB := &entities.Service{}

	var callsA, callsB []*net.IPNet
	cbA := func(ipNet *net.IPNet, _ bool) { callsA = append(callsA, ipNet) }
	cbB := func(ipNet *net.IPNet, _ bool) { callsB = append(callsB, ipNet) }

	// Mimic the closure getInterceptIP registers via resolver.AddDomain.
	wildcardLambdaA := func(host string) (net.IP, error) {
		return getDnsIp(host, cbA, svcA, resolver)
	}

	ipA, err := wildcardLambdaA("host.example")
	req.NoError(err)
	req.Len(callsA, 1, "wildcard lambda must invoke addrCB for the first service")

	// A second service joining the same hostname directly must reuse the
	// allocation and still get its own addrCB call.
	ipB := addInterceptHostname(t, "host.example", cbB, svcB, resolver)
	req.True(ipA.Equal(ipB), "second service must reuse the allocation from the wildcard lambda")
	req.Len(callsB, 1, "addrCB must fire so the joining service gets its iptables rule")

	// First cleanup must not recycle while another service still holds the hostname.
	svcA.RunCleanupActions()
	req.Equal(0, dnsRecycledIps.Len(), "IP must not be recycled while another service uses it")
	_, found := resolver.LookupIP("host.example.")
	req.True(found, "hostname must remain while another service still uses it")

	// Last cleanup releases the IP.
	svcB.RunCleanupActions()
	req.Equal(1, dnsRecycledIps.Len(), "IP must be recycled when the last service cleans up")
}

// Test_GetDnsIp_WildcardThenLiteral_RemoveLiteralFirst covers the other
// removal order for overlapping wildcard and literal intercepts: a hostname
// (test.example.com) is first allocated through a wildcard domain's callback
// (*.example.com), then a service intercepting the literal hostname joins.
// Removing the literal service must not remove the resolver entry or recycle
// the IP while the wildcard service's iptables rule still points at it.
// Before the fix the wildcard allocation held no reference, so the literal
// service's cleanup tore down the shared entry and recycled the IP.
func Test_GetDnsIp_WildcardThenLiteral_RemoveLiteralFirst(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := dns.NewRefCountingResolver(newFakeResolver())
	wildcardSvc := &entities.Service{}
	literalSvc := &entities.Service{}
	noopCB := func(*net.IPNet, bool) {}

	// wildcard path: a DNS query for test.example.com matches *.example.com
	// and the domain callback allocates and registers the hostname
	wildcardIP, err := getDnsIp("test.example.com", noopCB, wildcardSvc, resolver)
	req.NoError(err)

	// literal path: a service intercepting test.example.com directly joins
	literalIP := addInterceptHostname(t, "test.example.com", noopCB, literalSvc, resolver)
	req.True(wildcardIP.Equal(literalIP), "literal service must reuse the wildcard allocation")

	literalSvc.RunCleanupActions()
	_, found := resolver.LookupIP("test.example.com.")
	req.True(found, "hostname must remain while the wildcard service still uses it")
	req.Equal(0, dnsRecycledIps.Len(), "IP must not be recycled while the wildcard service uses it")

	wildcardSvc.RunCleanupActions()
	_, found = resolver.LookupIP("test.example.com.")
	req.False(found, "hostname must be removed when the last user cleans up")
	req.Equal(1, dnsRecycledIps.Len(), "IP must be recycled when the last user cleans up")
}

// recordingAddrCB collects the InterceptAddress values produced by
// GetInterceptAddresses so tests can assert per-service expansion.
type recordingAddrCB struct {
	calls []*InterceptAddress
}

func (r *recordingAddrCB) Apply(a *InterceptAddress) { r.calls = append(r.calls, a) }

// newSharedHostnameService builds a minimally populated entities.Service that
// intercepts the given hostname on a single TCP port. Only the fields
// GetInterceptAddresses actually reads are set.
func newSharedHostnameService(name, hostname string, port uint16) *entities.Service {
	svc := &entities.Service{}
	svc.Name = &name
	svc.InterceptV1Config = &entities.InterceptV1Config{
		Addresses:  []string{hostname},
		PortRanges: []*entities.PortRange{{Low: port, High: port}},
	}
	return svc
}

// Test_GetInterceptAddresses_SharedHostnameProducesRulePerService is the
// integration-level guard for #3867. GetInterceptAddresses is the actual entry
// point tproxy_linux.go calls per service; each Apply call eventually becomes
// an iptables rule. Before the fix the second service's callback never fired,
// so this test would observe zero InterceptAddress instances for svcB. It also
// guards against future drift in getInterceptIP's wiring that bypasses our
// finer-grained getDnsIp tests.
func Test_GetInterceptAddresses_SharedHostnameProducesRulePerService(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := dns.NewRefCountingResolver(newFakeResolver())
	svcA := newSharedHostnameService("svcA", "shared.example", 80)
	svcB := newSharedHostnameService("svcB", "shared.example", 80)

	cbA := &recordingAddrCB{}
	cbB := &recordingAddrCB{}

	req.NoError(GetInterceptAddresses(svcA, []string{"tcp"}, resolver, cbA))
	req.NoError(GetInterceptAddresses(svcB, []string{"tcp"}, resolver, cbB))

	req.Len(cbA.calls, 1, "svcA must receive its InterceptAddress")
	req.Len(cbB.calls, 1, "svcB must receive its InterceptAddress (the #3867 regression)")

	req.True(cbA.calls[0].IpNet().IP.Equal(cbB.calls[0].IpNet().IP),
		"both services must see the same allocated IP")
	req.Equal(uint16(80), cbA.calls[0].LowPort())
	req.Equal(uint16(80), cbB.calls[0].LowPort())
	req.Equal("tcp", cbA.calls[0].Proto())
	req.Equal("tcp", cbB.calls[0].Proto())
}

// Test_GetInterceptAddresses_SharedHostnameMultiPortProtocol checks that the
// per-service expansion of protocols x port ranges still fires correctly for
// the reuse path. With N services, M protocols, and K port ranges we expect
// each service to produce M*K InterceptAddress instances, all sharing the
// allocated IP.
func Test_GetInterceptAddresses_SharedHostnameMultiPortProtocol(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := dns.NewRefCountingResolver(newFakeResolver())

	makeSvc := func(name string) *entities.Service {
		n := name
		return &entities.Service{
			ServiceDetail: rest_model.ServiceDetail{
				BaseEntity: rest_model.BaseEntity{},
				Name:       &n,
			},
			InterceptV1Config: &entities.InterceptV1Config{
				Addresses:  []string{"multi.example"},
				PortRanges: []*entities.PortRange{{Low: 80, High: 80}, {Low: 443, High: 443}},
			},
		}
	}

	svcA, svcB := makeSvc("svcA"), makeSvc("svcB")
	cbA, cbB := &recordingAddrCB{}, &recordingAddrCB{}
	protocols := []string{"tcp", "udp"}

	req.NoError(GetInterceptAddresses(svcA, protocols, resolver, cbA))
	req.NoError(GetInterceptAddresses(svcB, protocols, resolver, cbB))

	// 2 protocols x 2 port ranges = 4 InterceptAddress per service.
	req.Len(cbA.calls, 4)
	req.Len(cbB.calls, 4, "reuse path must still expand protocols x port ranges for the joining service")

	for _, c := range cbB.calls {
		req.True(cbA.calls[0].IpNet().IP.Equal(c.IpNet().IP), "svcB calls must all use the shared IP")
	}
}
