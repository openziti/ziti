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

	"github.com/openziti/ziti/v2/tunnel/entities"
	"github.com/stretchr/testify/require"
)

// fakeResolver is a minimal in-memory dns.Resolver for testing getDnsIp.
// Only the methods getDnsIp actually exercises are implemented; the rest panic
// so an accidental dependency on more behavior fails loudly.
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

func (r *fakeResolver) RemoveHostname(hostname string) net.IP {
	key := strings.ToLower(hostname) + "."
	ip, ok := r.names[key]
	if !ok {
		return nil
	}
	delete(r.names, key)
	return ip
}

func (r *fakeResolver) addHostnameForTest(hostname string, ip net.IP) {
	r.names[strings.ToLower(hostname)+"."] = ip
}

func (r *fakeResolver) AddHostname(string, net.IP) error              { panic("unused") }
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
	dnsCurrentIpMtx.Lock()
	defer dnsCurrentIpMtx.Unlock()
	for k := range dnsHostRefCount {
		delete(dnsHostRefCount, k)
	}
}

// Test_GetDnsIp_SharedHostnameInstallsRule asserts that when two services share
// an intercept hostname, getDnsIp invokes addrCB for both services so each one
// gets its iptables rule installed (the regression in #3867). Without the fix
// the second service silently has no rule programmed and is unreachable.
func Test_GetDnsIp_SharedHostnameInstallsRule(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := newFakeResolver()
	svcA := &entities.Service{}
	svcB := &entities.Service{}

	var callsA, callsB []*net.IPNet
	cbA := func(ipNet *net.IPNet, _ bool) { callsA = append(callsA, ipNet) }
	cbB := func(ipNet *net.IPNet, _ bool) { callsB = append(callsB, ipNet) }

	ipA, err := getDnsIp("shared.example", cbA, svcA, resolver)
	req.NoError(err)
	req.Len(callsA, 1, "addrCB must fire for the first service")
	req.True(ipA.Equal(callsA[0].IP), "addrCB receives the allocated IP")

	// Simulate the side effect getInterceptIP performs after getDnsIp returns:
	// register the hostname in the resolver. getDnsIp's LookupIP for the next
	// caller depends on this.
	resolver.addHostnameForTest("shared.example", ipA)

	ipB, err := getDnsIp("shared.example", cbB, svcB, resolver)
	req.NoError(err)
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

	resolver := newFakeResolver()
	svcA := &entities.Service{}
	svcB := &entities.Service{}
	noopCB := func(*net.IPNet, bool) {}

	ipA, err := getDnsIp("shared.example", noopCB, svcA, resolver)
	req.NoError(err)
	resolver.addHostnameForTest("shared.example", ipA)

	_, err = getDnsIp("shared.example", noopCB, svcB, resolver)
	req.NoError(err)

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
// hostnames differ only in case share a single allocation and refcount entry,
// matching the resolver's own case-insensitive view.
func Test_GetDnsIp_SharedHostnameDifferentCase(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := newFakeResolver()
	svcA := &entities.Service{}
	svcB := &entities.Service{}
	noopCB := func(*net.IPNet, bool) {}

	ipA, err := getDnsIp("Shared.Example", noopCB, svcA, resolver)
	req.NoError(err)
	resolver.addHostnameForTest("shared.example", ipA)

	ipB, err := getDnsIp("shared.example", noopCB, svcB, resolver)
	req.NoError(err)
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
// same hostname. Before refcounting, the first service's cleanUpFunc was the
// only one registered for a shared hostname and would unconditionally remove
// the resolver entry and recycle the IP.
func Test_GetDnsIp_CleanupOutOfOrder(t *testing.T) {
	req := require.New(t)
	resetDnsState(t)

	resolver := newFakeResolver()
	svcA := &entities.Service{}
	svcB := &entities.Service{}
	noopCB := func(*net.IPNet, bool) {}

	ipA, err := getDnsIp("shared.example", noopCB, svcA, resolver)
	req.NoError(err)
	resolver.addHostnameForTest("shared.example", ipA)

	_, err = getDnsIp("shared.example", noopCB, svcB, resolver)
	req.NoError(err)

	svcA.RunCleanupActions()
	_, found := resolver.LookupIP("shared.example.")
	req.True(found, "hostname must remain when an earlier-registered service cleans up while others still use it")
	req.Equal(0, dnsRecycledIps.Len(), "IP must not be recycled while another service uses it")
}
