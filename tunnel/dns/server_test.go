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

package dns

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestResolver builds a resolver with no listening DNS server, suitable
// for exercising getAddress directly.
func newTestResolver() *resolver {
	return &resolver{
		names:   map[string]net.IP{},
		ips:     map[string]string{},
		domains: map[string]*domainEntry{},
	}
}

// Test_GetAddress_DomainCallbackRegistersHostname pins the AddDomain callback
// contract: the callback registers the hostname -> IP mapping itself (through
// the refcounting layer in production), and getAddress does not add it to the
// wrapped resolver directly. A direct add would bypass the refcount and let
// one service's cleanup remove a hostname other services still use.
func Test_GetAddress_DomainCallbackRegistersHostname(t *testing.T) {
	req := require.New(t)

	wrapped := newTestResolver()
	wrapper := NewRefCountingResolver(wrapped)

	ip := net.IP{100, 64, 0, 1}
	calls := 0
	req.NoError(wrapper.AddDomain("*.example.com", func(host string) (net.IP, error) {
		calls++
		req.NoError(wrapper.AddHostname(host, ip))
		return ip, nil
	}))

	got, err := wrapped.getAddress("test.example.com.")
	req.NoError(err)
	req.True(ip.Equal(got))
	req.Equal(1, calls, "first query must invoke the domain callback")

	got, err = wrapped.getAddress("test.example.com.")
	req.NoError(err)
	req.True(ip.Equal(got))
	req.Equal(1, calls, "registered hostname must short-circuit; callback must not run again")

	// the callback registered through the refcounting layer, so a paired
	// release removes the mapping from the wrapped resolver
	req.NotNil(wrapper.RemoveHostname("test.example.com"))
	_, found := wrapped.LookupIP("test.example.com.")
	req.False(found, "hostname must be removed once its only reference is released")
}
